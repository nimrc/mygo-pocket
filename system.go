package main

// The handheld's state and controls, through ROCKNIX's own tools and settings (system.cfg through get_setting / set_setting in
// /etc/profile.d/001-functions), so EmulationStation and this launcher keep the same settings.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// run runs a command with a deadline and returns its trimmed output (bluetoothctl can block for good without its daemon).
func run(timeout time.Duration, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(out))
}

func shell(timeout time.Duration, script string) string { return run(timeout, "bash", "-c", script) }

func getSetting(key string) string {
	return shell(3*time.Second, ". /etc/profile.d/001-functions; get_setting "+key)
}

func setSetting(key, value string) {
	shell(3*time.Second, fmt.Sprintf(". /etc/profile.d/001-functions; set_setting %s %q", key, value))
}

func readFile(path string) string { b, _ := os.ReadFile(path); return strings.TrimSpace(string(b)) }
func readInt(path string) int     { n, _ := strconv.Atoi(readFile(path)); return n }

// ---- battery, temperatures ----

type battery struct {
	percent  int
	charging bool
}

func readBattery() battery {
	return battery{readInt("/sys/class/power_supply/battery/capacity"), readFile("/sys/class/power_supply/battery/status") == "Charging"}
}

// hottest returns the highest temperature in degrees Celsius among the thermal zones whose type starts with prefix.
func hottest(prefix string) int {
	best := 0
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		if strings.HasPrefix(readFile(z+"/type"), prefix) {
			best = max(best, readInt(z+"/temp")/1000)
		}
	}
	return best
}

// ---- brightness (ROCKNIX's brightness tool keeps its setting) ----

func backlight() string {
	m, _ := filepath.Glob("/sys/class/backlight/*")
	if len(m) == 0 {
		return ""
	}
	return m[0]
}

func readBrightness() int {
	b := backlight()
	if mx := readInt(b + "/max_brightness"); mx > 0 {
		return (readInt(b+"/brightness")*100 + mx/2) / mx
	}
	return 0
}

func setBrightness(percent int) { run(3*time.Second, "brightness", "set", strconv.Itoa(clamp(percent, 5, 100))) }

// ---- volume (PipeWire's default sink) ----

func readVolume() (percent int, muted bool) {
	out := run(2*time.Second, "wpctl", "get-volume", "@DEFAULT_AUDIO_SINK@") // "Volume: 0.30 [MUTED]"
	f := strings.Fields(out)
	if len(f) >= 2 {
		v, _ := strconv.ParseFloat(f[1], 64)
		percent = int(v*100 + 0.5)
	}
	return percent, strings.Contains(out, "MUTED")
}

func setVolume(percent int) {
	percent = clamp(percent, 0, 100)
	run(2*time.Second, "wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", fmt.Sprintf("%d%%", percent))
	setSetting("audio.volume", strconv.Itoa(percent))
}

func toggleMute() { run(2*time.Second, "wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle") }

// ---- Wi-Fi (NetworkManager) ----

type wifiNet struct {
	ssid   string
	signal int
	secure bool
	inUse  bool
	saved  bool
}

type wifiState struct {
	enabled bool
	ssid    string
	ip      string
}

func readWifi() wifiState {
	s := wifiState{enabled: run(2*time.Second, "nmcli", "radio", "wifi") == "enabled"}
	for _, line := range strings.Split(run(2*time.Second, "nmcli", "-t", "-f", "GENERAL.CONNECTION,IP4.ADDRESS", "dev", "show", "wlan0"), "\n") {
		k, v, _ := strings.Cut(line, ":")
		switch {
		case k == "GENERAL.CONNECTION" && v != "--":
			s.ssid = v
		case strings.HasPrefix(k, "IP4.ADDRESS") && s.ip == "":
			s.ip, _, _ = strings.Cut(v, "/")
		}
	}
	return s
}

func setWifi(on bool) {
	state, flag := "off", "0"
	if on {
		state, flag = "on", "1"
	}
	run(5*time.Second, "nmcli", "radio", "wifi", state)
	setSetting("wifi.enabled", flag)
}

// scanWifi lists the networks in reach, strongest first, marking the ones NetworkManager has a profile for; cachedWifi lists NetworkManager's
// last scan without a new one (fast: the full refresh reads it for the signal bars)
func scanWifi() []wifiNet { return wifiList("yes", 15*time.Second) }
func cachedWifi() []wifiNet { return wifiList("no", 3*time.Second) }

// markInUse marks the network wlan0 is on: nmcli's list keeps its IN-USE mark from its last scan, after a connection changed
func markInUse(nets []wifiNet, ssid string) []wifiNet {
	for i := range nets {
		nets[i].inUse = ssid != "" && nets[i].ssid == ssid
	}
	sort.SliceStable(nets, func(i, j int) bool { return nets[i].inUse && !nets[j].inUse })
	return nets
}

func wifiList(rescan string, timeout time.Duration) []wifiNet {
	saved := map[string]bool{}
	for _, line := range strings.Split(run(3*time.Second, "nmcli", "-t", "-f", "NAME,TYPE", "con", "show"), "\n") {
		if name, typ, ok := strings.Cut(line, ":"); ok && strings.Contains(typ, "wireless") {
			saved[name] = true
		}
	}
	seen := map[string]bool{}
	var nets []wifiNet
	out := run(timeout, "nmcli", "-t", "-f", "IN-USE,SSID,SIGNAL,SECURITY", "dev", "wifi", "list", "--rescan", rescan)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.ReplaceAll(line, `\:`, "\x00"), ":")
		if len(f) < 4 || f[1] == "" {
			continue
		}
		ssid := strings.ReplaceAll(f[1], "\x00", ":")
		if seen[ssid] {
			continue
		}
		seen[ssid] = true
		sig, _ := strconv.Atoi(f[2])
		nets = append(nets, wifiNet{ssid: ssid, signal: sig, secure: f[3] != "" && f[3] != "--", inUse: f[0] == "*", saved: saved[ssid]})
	}
	sort.SliceStable(nets, func(i, j int) bool { // the one in use first, then the strongest
		if nets[i].inUse != nets[j].inUse {
			return nets[i].inUse
		}
		return nets[i].signal > nets[j].signal
	})
	return nets
}

// connectWifi joins a saved or open network; a new secured one needs its password (a text entry this launcher does not have yet).
func connectWifi(n wifiNet) string {
	if n.saved {
		return run(30*time.Second, "nmcli", "con", "up", "id", n.ssid)
	}
	if !n.secure {
		return run(30*time.Second, "nmcli", "dev", "wifi", "connect", n.ssid)
	}
	return "需要密码"
}

// ---- Bluetooth (ROCKNIX's rocknix-bluetooth starts and stops the daemon and its agent) ----

type btDevice struct {
	mac, name string
	connected bool
}

func bluetoothOn() bool { return run(2*time.Second, "systemctl", "is-active", "bluetooth") == "active" }

func setBluetooth(on bool) {
	if on {
		run(15*time.Second, "rocknix-bluetooth", "enable")
	} else {
		run(15*time.Second, "rocknix-bluetooth", "disable")
	}
}

func btList(filter string) map[string]string {
	out := map[string]string{}
	args := []string{"--timeout", "2", "devices"}
	if filter != "" { // an empty filter argument lists nothing
		args = append(args, filter)
	}
	for _, line := range strings.Split(run(4*time.Second, "bluetoothctl", args...), "\n") {
		f := strings.SplitN(line, " ", 3) // Device <mac> <name>
		if len(f) == 3 && f[0] == "Device" {
			out[f[1]] = f[2]
		}
	}
	return out
}

func readBtDevices() []btDevice {
	connected := btList("Connected")
	var ds []btDevice
	for mac, name := range btList("Paired") {
		ds = append(ds, btDevice{mac, name, connected[mac] != ""})
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].name < ds[j].name })
	return ds
}

func toggleBtDevice(d btDevice) {
	if d.connected {
		run(10*time.Second, "bluetoothctl", "--timeout", "8", "disconnect", d.mac)
	} else {
		run(15*time.Second, "rocknix-bluetooth", "connect", d.mac)
	}
}

// ---- fan (ROCKNIX's fancontrol sets the thermal zones' trip points for a profile and exits) ----

var fanProfiles = []string{"quiet", "moderate", "aggressive"}
var fanNames = map[string]string{"quiet": "安静", "moderate": "均衡", "aggressive": "强劲"}

func readFanProfile() string {
	p := getSetting("cooling.profile")
	if p == "" {
		return "moderate"
	}
	return p
}

func setFanProfile(p string) {
	setSetting("cooling.profile", p)
	shell(10*time.Second, ". /etc/profile; fancontrol")
}

func fanSpeed() int {
	m, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, h := range m {
		if readFile(h+"/name") == "pwmfan" {
			return readInt(h+"/pwm1") * 100 / 255
		}
	}
	return -1
}

// ---- CPU governor (all clusters; system.cpugovernor is what ROCKNIX applies at boot) ----

var governors = []string{"schedutil", "ondemand", "performance", "powersave"}
var governorNames = map[string]string{"schedutil": "调度器 schedutil", "ondemand": "按需 ondemand", "performance": "性能 performance", "powersave": "省电 powersave"}

func readGovernor() string {
	return readFile("/sys/devices/system/cpu/cpufreq/policy0/scaling_governor")
}

func setGovernor(g string) {
	policies, _ := filepath.Glob("/sys/devices/system/cpu/cpufreq/policy*")
	for _, p := range policies {
		os.WriteFile(p+"/scaling_governor", []byte(g), 0)
	}
	setSetting("system.cpugovernor", g)
}

// ---- system ----

func sshOn() bool { return run(2*time.Second, "systemctl", "is-active", "sshd") == "active" }

func setSSH(on bool) {
	if on {
		setSetting("ssh.enabled", "1")
		run(10*time.Second, "systemctl", "start", "sshd")
	} else {
		setSetting("ssh.enabled", "0")
		run(10*time.Second, "systemctl", "stop", "sshd")
	}
}

func storageFree() string {
	var st syscall.Statfs_t
	if syscall.Statfs("/storage", &st) != nil {
		return "?"
	}
	free, total := st.Bavail*uint64(st.Bsize), st.Blocks*uint64(st.Bsize)
	return fmt.Sprintf("%.1f / %.1f GB", float64(free)/1e9, float64(total)/1e9)
}

func memoryFree() string {
	for _, line := range strings.Split(readFile("/proc/meminfo"), "\n") {
		if strings.HasPrefix(line, "MemAvailable:") {
			kb, _ := strconv.Atoi(strings.Fields(line)[1])
			return fmt.Sprintf("%.1f GB 可用", float64(kb)/1e6)
		}
	}
	return "?"
}

func osVersion() string {
	for _, line := range strings.Split(readFile("/etc/os-release"), "\n") {
		if v, ok := strings.CutPrefix(line, "OS_VERSION="); ok {
			return "ROCKNIX " + strings.Trim(v, `"`)
		}
	}
	return "ROCKNIX"
}

func clamp(v, lo, hi int) int { return max(lo, min(hi, v)) }

// ---- Wi-Fi: joining with a password, forgetting ----

// joinWifi joins a secured network with its password. A failed join leaves NetworkManager a profile with the wrong secret, which would
// make the network look saved: it goes, and NetworkManager goes back to the network it had
func joinWifi(ssid, password string) string {
	out := run(40*time.Second, "nmcli", "dev", "wifi", "connect", ssid, "password", password)
	if strings.Contains(out, "successfully activated") {
		return "已连接 " + ssid
	}
	run(10*time.Second, "nmcli", "con", "delete", "id", ssid)
	return "连接 " + ssid + " 失败：" + wifiError(out)
}

// wifiError says why nmcli could not join, in the launcher's words
func wifiError(out string) string {
	switch o := strings.ToLower(out); {
	case strings.Contains(o, "secrets were required"), strings.Contains(o, "psk"), strings.Contains(o, "authentication"), strings.Contains(o, "password"):
		return "密码错误"
	case strings.Contains(o, "no network with ssid"):
		return "找不到这个网络，请重新扫描"
	case strings.Contains(o, "timeout"), strings.Contains(o, "timed out"):
		return "连接超时"
	case out == "":
		return "没有响应"
	}
	return firstLine(out)
}

func forgetWifi(ssid string) string { return run(10*time.Second, "nmcli", "con", "delete", "id", ssid) }

// ---- Bluetooth: finding and pairing new devices ----

// scanBluetooth looks for devices for a few seconds and returns the ones not paired yet
func scanBluetooth() []btDevice {
	run(14*time.Second, "bluetoothctl", "--timeout", "10", "scan", "on")
	paired := btList("Paired")
	var ds []btDevice
	for mac, name := range btList("") {
		if paired[mac] == "" && name != strings.ReplaceAll(mac, ":", "-") {
			ds = append(ds, btDevice{mac: mac, name: name})
		}
	}
	sort.Slice(ds, func(i, j int) bool { return ds[i].name < ds[j].name })
	return ds
}

func pairBluetooth(d btDevice) string {
	out := run(30*time.Second, "bluetoothctl", "--timeout", "25", "pair", d.mac)
	run(5*time.Second, "bluetoothctl", "--timeout", "3", "trust", d.mac)
	run(15*time.Second, "bluetoothctl", "--timeout", "10", "connect", d.mac)
	return out
}

func forgetBluetooth(d btDevice) string {
	return run(10*time.Second, "bluetoothctl", "--timeout", "5", "remove", d.mac)
}

// ---- audio output: the speaker or the headphones (ROCKNIX's audio.device) ----

type sink struct {
	id   string
	name string
	def  bool
}

func readSinks() []sink {
	var out []sink
	in := false
	for _, line := range strings.Split(run(3*time.Second, "wpctl", "status"), "\n") {
		t := strings.Trim(line, " │├└─")
		switch {
		case strings.HasPrefix(t, "Sinks:"):
			in = len(out) == 0 // the first Sinks list is the audio one
		case strings.HasSuffix(t, ":"):
			in = false
		case in && t != "":
			def := strings.HasPrefix(t, "*")
			t = strings.TrimSpace(strings.TrimPrefix(t, "*"))
			id, rest, ok := strings.Cut(t, ". ")
			if !ok {
				continue
			}
			name, _, _ := strings.Cut(rest, "[")
			out = append(out, sink{id, strings.TrimSpace(name), def})
		}
	}
	return out
}


// ---- GPU governor (devfreq) ----

var gpuGovernors = []string{"simple_ondemand", "performance", "powersave"}
var gpuGovernorNames = map[string]string{"simple_ondemand": "按需", "performance": "性能", "powersave": "省电"}

func gpuDevfreq() string { m, _ := filepath.Glob("/sys/class/devfreq/*.gpu"); if len(m) == 0 { return "" }; return m[0] }
func readGPUGovernor() string { return readFile(gpuDevfreq() + "/governor") }
func setGPUGovernor(g string) { os.WriteFile(gpuDevfreq()+"/governor", []byte(g), 0) }
func gpuFreq() int            { return readInt(gpuDevfreq()+"/cur_freq") / 1000000 }

func cpuFreqs() string {
	var parts []string
	policies, _ := filepath.Glob("/sys/devices/system/cpu/cpufreq/policy*")
	sort.Strings(policies)
	for _, p := range policies {
		parts = append(parts, strconv.Itoa(readInt(p+"/scaling_cur_freq")/1000))
	}
	return strings.Join(parts, " / ") + " MHz"
}

// ---- services: Samba, Syncthing, network time ----

func serviceOn(name string) bool { return run(2*time.Second, "systemctl", "is-active", name) == "active" }

func setService(name, settingKey string, on bool) {
	flag, verb := "0", "stop"
	if on {
		flag, verb = "1", "start"
	}
	if settingKey != "" {
		setSetting(settingKey, flag)
	}
	run(20*time.Second, "systemctl", verb, name)
}

// ---- power: battery details ----

type batteryInfo struct {
	voltage float64 // V
	current float64 // A, positive charging
	health  string
	temp    float64 // °C
}

func readBatteryInfo() batteryInfo {
	b := "/sys/class/power_supply/battery/"
	return batteryInfo{float64(readInt(b+"voltage_now")) / 1e6, float64(readInt(b+"current_now")) / 1e6, readFile(b + "health"), float64(readInt(b+"temp")) / 10}
}

// ---- system: time zone, host, uptime ----

var timezones = []string{"Asia/Shanghai", "Asia/Hong_Kong", "Asia/Taipei", "Asia/Tokyo", "Asia/Singapore", "Europe/London", "Europe/Berlin", "America/New_York", "America/Los_Angeles", "UTC"}

func readTimezone() string {
	if t := getSetting("system.timezone"); t != "" {
		return t
	}
	return "UTC"
}

func setTimezone(tz string) string { return run(15*time.Second, "/usr/bin/steamos-polkit-helpers/steamos-set-timezone", tz) }

func uptime() string {
	f := strings.Fields(readFile("/proc/uptime"))
	if len(f) == 0 {
		return "?"
	}
	s, _ := strconv.ParseFloat(f[0], 64)
	d := time.Duration(s) * time.Second
	return fmt.Sprintf("%d 小时 %d 分", int(d.Hours()), int(d.Minutes())%60)
}

func kernel() string { return run(2*time.Second, "uname", "-r") }
