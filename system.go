package main

// The handheld's state and controls, through ROCKNIX's own tools and settings (system.cfg through get_setting / set_setting in
// /etc/profile.d/001-functions), so EmulationStation and this launcher keep the same settings.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\x01|\x02`) // bluetoothctl colours its output and wraps it in readline markers

// run runs a command with a deadline and returns its trimmed output (bluetoothctl can block for good without its daemon).
var run = func(timeout time.Duration, name string, args ...string) string { // a var: the tests swap in a fake handheld
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(ansi.ReplaceAllString(string(out), ""))
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
	ssid       string
	signal     int
	secure     bool
	enterprise bool // 802.1X: an account and a password, which iwd takes from a provisioning file
	inUse      bool
	saved      bool
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
	saved := wifiSaved()
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
		nets = append(nets, wifiNet{ssid: ssid, signal: sig, secure: f[3] != "" && f[3] != "--", enterprise: strings.Contains(f[3], "802.1X"),
			inUse: f[0] == "*", saved: saved[ssid]})
	}
	sort.SliceStable(nets, func(i, j int) bool { // the one in use first, then the strongest
		if nets[i].inUse != nets[j].inUse {
			return nets[i].inUse
		}
		return nets[i].signal > nets[j].signal
	})
	return nets
}

// connectWifi joins a saved or open network; when it cannot, wlan0 goes back to the network it was on
func connectWifi(n wifiNet) string {
	prev := readWifi().ssid
	var out string
	if n.saved {
		out = run(45*time.Second, "nmcli", "-w", "30", "con", "up", "id", n.ssid)
	} else {
		out = run(45*time.Second, "nmcli", "-w", "30", "dev", "wifi", "connect", n.ssid)
	}
	if strings.Contains(out, "successfully activated") {
		return "已连接 " + n.ssid
	}
	if !n.saved {
		run(10*time.Second, "nmcli", "con", "delete", "id", n.ssid)
	}
	return "连接 " + n.ssid + " 失败：" + wifiError(out) + goBack(prev, n.ssid)
}

// goBack rejoins the network wlan0 was on before a join that failed, which left it on none (iwd does not go back by itself), and says so
func goBack(prev, tried string) string {
	if prev == "" || prev == tried {
		return ""
	}
	if strings.Contains(run(45*time.Second, "nmcli", "-w", "30", "con", "up", "id", prev), "successfully activated") {
		return "，已回到 " + prev
	}
	return ""
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

// btInfo reads a device's yes/no fields from bluetoothctl info ("Paired", "Connected", ...)
func btInfo(mac string) map[string]bool {
	out := map[string]bool{}
	for _, line := range strings.Split(run(5*time.Second, "bluetoothctl", "--timeout", "3", "info", mac), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), ": "); ok && (v == "yes" || v == "no") {
			out[k] = v == "yes"
		}
	}
	return out
}

// toggleBtDevice connects a paired device or disconnects it, and says which it did
func toggleBtDevice(d btDevice) string {
	if d.connected {
		run(10*time.Second, "bluetoothctl", "--timeout", "8", "disconnect", d.mac)
		if !btInfo(d.mac)["Connected"] {
			return "已断开 " + d.name
		}
		return "断开 " + d.name + " 失败"
	}
	run(15*time.Second, "rocknix-bluetooth", "connect", d.mac)
	if btInfo(d.mac)["Connected"] {
		return "已连接 " + d.name
	}
	return "连接 " + d.name + " 失败：设备没有响应"
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
// make the network look saved: it goes, and wlan0 goes back to the network it was on. With iwd, a wrong password does not fail: nmcli
// waits until its timeout.
func joinWifi(ssid, password string) string {
	prev := readWifi().ssid
	out := run(45*time.Second, "nmcli", "-w", "30", "dev", "wifi", "connect", ssid, "password", password)
	if strings.Contains(out, "successfully activated") {
		return "已连接 " + ssid
	}
	run(10*time.Second, "nmcli", "con", "delete", "id", ssid)
	return "连接 " + ssid + " 失败：" + wifiError(out) + goBack(prev, ssid)
}

// wifiError says why nmcli could not join, in the launcher's words
func wifiError(out string) string {
	switch o := strings.ToLower(out); {
	case strings.Contains(o, "secrets were required"), strings.Contains(o, "psk"), strings.Contains(o, "authentication"), strings.Contains(o, "password"):
		return "密码错误"
	case strings.Contains(o, "no network with ssid"):
		return "找不到这个网络，请重新扫描"
	case strings.Contains(o, "timeout"), strings.Contains(o, "timed out"), out == "":
		return "密码错误或信号太弱"
	}
	return firstLine(out)
}

// forgetWifi deletes the network's profile; NetworkManager has iwd forget it too, which deletes an 802.1X network's provisioning file,
// so its copy goes as well
func forgetWifi(ssid string) string {
	os.Remove(filepath.Join(provisionCopies, provisionFile(ssid)))
	out := run(10*time.Second, "nmcli", "con", "delete", "id", ssid)
	if strings.Contains(out, "successfully deleted") {
		return "已忘记 " + ssid
	}
	return out
}

// ---- Wi-Fi: 802.1X (enterprise) networks ----
//
// NetworkManager's iwd backend joins an 802.1X network only from iwd's provisioning file, and its profile of the network is a mirror of
// that file in /run. iwd loses the file when it restarts (NetworkManager has it forget the network as the mirror goes), and the network
// is gone for good. The launcher keeps a copy of each provisioning file and puts back one that went missing.

var (
	iwdState        = "/storage/.cache/iwd"
	provisionCopies = "/storage/.config/mygo-pocket/iwd"
)

// provisionFile is iwd's file name for an 802.1X network: the SSID when it is plain, "=" and its hex otherwise
func provisionFile(ssid string) string {
	for _, r := range ssid {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ' || r == '-' || r == '_') {
			return fmt.Sprintf("=%x.8021x", ssid)
		}
	}
	return ssid + ".8021x"
}

// joinEnterprise joins an 802.1X network with an account and a password, over PEAP with MSCHAPv2 (what most offices run), and keeps
// a copy of its provisioning file; a failed join leaves neither
func joinEnterprise(ssid, identity, password string) string {
	prev := readWifi().ssid
	f := provisionFile(ssid)
	conf := fmt.Sprintf("[Security]\nEAP-Method=PEAP\nEAP-Identity=%s\nEAP-PEAP-Phase2-Method=MSCHAPV2\nEAP-PEAP-Phase2-Identity=%s\nEAP-PEAP-Phase2-Password=%s\n\n[Settings]\nAutoConnect=true\n",
		identity, identity, password)
	if err := os.WriteFile(filepath.Join(iwdState, f), []byte(conf), 0o600); err != nil {
		return "连接 " + ssid + " 失败：" + err.Error()
	}
	for range 10 { // NetworkManager mirrors the new file into a profile
		if wifiSaved()[ssid] {
			break
		}
		time.Sleep(time.Second)
	}
	out := run(45*time.Second, "nmcli", "-w", "30", "con", "up", "id", ssid)
	if strings.Contains(out, "successfully activated") {
		os.MkdirAll(provisionCopies, 0o700)
		os.WriteFile(filepath.Join(provisionCopies, f), []byte(conf), 0o600)
		return "已连接 " + ssid
	}
	os.Remove(filepath.Join(iwdState, f))
	return "连接 " + ssid + " 失败：账号或密码错误，或不是 PEAP 网络" + goBack(prev, ssid)
}

// keepProvisioning copies new or changed 802.1X provisioning files aside and puts back the ones iwd lost; it returns the networks put
// back. inUse is the network wlan0 is on: the last 802.1X one is remembered, and when it is put back while iwd has meanwhile joined
// another saved network, it is joined again (rejoin).
func keepProvisioning(inUse string) (back []string, rejoin string) {
	files, _ := filepath.Glob(filepath.Join(iwdState, "*.8021x"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		c := filepath.Join(provisionCopies, filepath.Base(f))
		if old, _ := os.ReadFile(c); string(old) != string(data) {
			os.MkdirAll(provisionCopies, 0o700)
			os.WriteFile(c, data, 0o600)
		}
	}
	last := filepath.Join(provisionCopies, ".in-use")
	if inUse != "" && exists(filepath.Join(provisionCopies, provisionFile(inUse))) && readFile(last) != inUse {
		os.WriteFile(last, []byte(inUse), 0o600)
	}
	copies, _ := filepath.Glob(filepath.Join(provisionCopies, "*.8021x"))
	for _, c := range copies {
		f := filepath.Join(iwdState, filepath.Base(c))
		if exists(f) {
			continue
		}
		if data, err := os.ReadFile(c); err == nil && os.WriteFile(f, data, 0o600) == nil {
			ssid := strings.TrimSuffix(filepath.Base(c), ".8021x")
			back = append(back, ssid)
			if ssid == readFile(last) && ssid != inUse {
				rejoin = ssid
			}
		}
	}
	return back, rejoin
}

// rejoinWifi joins a network put back once NetworkManager has mirrored its provisioning file
func rejoinWifi(ssid string) string {
	for range 10 {
		if wifiSaved()[ssid] {
			break
		}
		time.Sleep(time.Second)
	}
	return run(60*time.Second, "nmcli", "-w", "45", "con", "up", "id", ssid)
}

func wifiSaved() map[string]bool {
	saved := map[string]bool{}
	for _, line := range strings.Split(run(3*time.Second, "nmcli", "-t", "-f", "NAME,TYPE", "con", "show"), "\n") {
		if name, typ, ok := strings.Cut(line, ":"); ok && strings.Contains(typ, "wireless") {
			saved[name] = true
		}
	}
	return saved
}

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

// pairBluetooth pairs, trusts (so it reconnects by itself) and connects a device, and says how far it got
func pairBluetooth(d btDevice) string {
	run(30*time.Second, "bluetoothctl", "--timeout", "25", "pair", d.mac)
	if !btInfo(d.mac)["Paired"] {
		return "配对 " + d.name + " 失败：请让设备进入配对模式后重试"
	}
	run(5*time.Second, "bluetoothctl", "--timeout", "3", "trust", d.mac)
	run(15*time.Second, "bluetoothctl", "--timeout", "10", "connect", d.mac)
	if btInfo(d.mac)["Connected"] {
		return "已配对并连接 " + d.name
	}
	return "已配对 " + d.name + "（未连接）"
}

func forgetBluetooth(d btDevice) string {
	run(10*time.Second, "bluetoothctl", "--timeout", "5", "remove", d.mac)
	if btList("Paired")[d.mac] == "" {
		return "已忘记 " + d.name
	}
	return "忘记 " + d.name + " 失败"
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
	voltage float64 // V, 0 where the gauge does not say
	current float64 // A, positive charging
	health  string
	temp    float64 // °C
	hasTemp bool
}

// readBatteryInfo reads the gauge's _now values, or the _avg ones the RG ARC's rk817 gives instead; it has no temperature
func readBatteryInfo() batteryInfo {
	b := "/sys/class/power_supply/battery/"
	first := func(names ...string) string {
		for _, n := range names {
			if v := readFile(b + n); v != "" {
				return v
			}
		}
		return ""
	}
	num := func(v string) float64 { f, _ := strconv.ParseFloat(v, 64); return f }
	temp := readFile(b + "temp")
	return batteryInfo{num(first("voltage_now", "voltage_avg")) / 1e6, num(first("current_now", "current_avg")) / 1e6, readFile(b + "health"),
		num(temp) / 10, temp != ""}
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

// ---- local netplay: ROCKNIX's ad-hoc network (wifictl), for RetroArch netplay without a network around ----
//
// The host (wifi.adhoc.id 1) opens the access point NETPLAY_AP at 192.168.80.1; a client joins it as 192.168.80.<id>. The addresses are
// static, so there is no DHCP to wait for.

const (
	netplaySSID = "NETPLAY_AP"
	netplayHost = "192.168.80.1"
)

const (
	netplayOff    = "off"
	netplayServe  = "host"
	netplayClient = "client"
)

var netplayModes = []string{netplayOff, netplayServe, netplayClient}

// rejoinSaved joins the network wlan0 was on before local netplay, then the strongest saved one in reach; it returns the one joined
func rejoinSaved() string {
	tried := map[string]bool{netplaySSID: true}
	try := []string{getSetting(netplayReturn)}
	for _, n := range cachedWifi() { // strongest first
		if n.saved {
			try = append(try, n.ssid)
		}
	}
	for _, ssid := range try {
		if ssid == "" || tried[ssid] {
			continue
		}
		tried[ssid] = true
		if strings.Contains(run(45*time.Second, "nmcli", "-w", "30", "con", "up", "id", ssid), "successfully activated") {
			return ssid
		}
	}
	return ""
}

// netplayReturn is the setting that keeps the network wlan0 was on before local netplay
const netplayReturn = "mygo-pocket.netplay.return"

var netplayNames = map[string]string{netplayOff: "关闭", netplayServe: "主机（开热点）", netplayClient: "加入主机"}

func readNetplay() string {
	if getSetting("wifi.adhoc.enabled") != "1" {
		return netplayOff
	}
	if getSetting("wifi.adhoc.id") == "1" {
		return netplayServe
	}
	return netplayClient
}

// clientID is this handheld's address on the host's network, 2–251 from its Wi-Fi MAC, so the clients of one host differ without a picker
func clientID() int {
	mac := readFile("/sys/class/net/wlan0/address")
	b, err := strconv.ParseUint(mac[max(0, len(mac)-2):], 16, 8)
	if err != nil {
		return 2
	}
	return 2 + int(b)%250
}

func setNetplay(mode string) string {
	if mode == netplayOff {
		run(30*time.Second, "wifictl", "disconnect") // while wifi.adhoc.enabled is still 1: deletes NETPLAY_AP
		setSetting("wifi.adhoc.enabled", "0")
		// A host's iwd goes back to rejoining saved networks by itself. A client's does not: NetworkManager asked it to disconnect as the
		// profile went, which stops its autoconnect. nmcli device connect fails on the iwd backend, and wifictl connect would take
		// wifi.ssid, which may be stale: the launcher rejoins the network it was on before, then the strongest saved one in reach.
		for range 5 {
			if w := readWifi(); w.ssid != "" && w.ssid != netplaySSID { // the access point's or host's network lingers a moment
				return "已退出本地联机 · " + w.ssid
			}
			time.Sleep(time.Second)
		}
		// After failed WPA handshakes with a host, the RTL8821CS (rtw88) has been seen to fail every association (status 1) until its
		// radio is turned off and on: that is the second round.
		for round := range 2 {
			if round == 1 {
				run(10*time.Second, "nmcli", "radio", "wifi", "off")
				time.Sleep(2 * time.Second)
				run(10*time.Second, "nmcli", "radio", "wifi", "on")
				for range 15 { // iwd rejoins a saved network by itself after the radio comes back
					time.Sleep(time.Second)
					if w := readWifi(); w.ssid != "" && w.ssid != netplaySSID {
						return "已退出本地联机 · " + w.ssid
					}
				}
			}
			if ssid := rejoinSaved(); ssid != "" {
				return "已退出本地联机 · " + ssid
			}
		}
		return "已退出本地联机，没有连上保存过的 Wi-Fi"
	}
	if w := readWifi(); w.ssid != "" && w.ssid != netplaySSID {
		setSetting(netplayReturn, w.ssid) // where leaving goes back to
	}
	id := 1
	if mode == netplayClient {
		id = clientID()
	}
	setWifi(true)
	setSetting("wifi.adhoc.enabled", "1")
	setSetting("wifi.adhoc.id", strconv.Itoa(id))
	run(150*time.Second, "wifictl", "connect") // a client scans for the host up to 30 s, then tries three times
	// joined only with an address on the host's network: while NetworkManager is still activating NETPLAY_AP it already names it, and
	// the access point's own address comes a moment after it starts
	in := func(w wifiState) bool { return w.ssid == netplaySSID && strings.HasPrefix(w.ip, "192.168.80.") }
	w := readWifi()
	for i := 0; i < 10 && !in(w); i++ {
		time.Sleep(time.Second)
		w = readWifi()
	}
	if !in(w) { // back to off, so the mode shown is the network in use
		back := strings.TrimPrefix(setNetplay(netplayOff), "已退出本地联机")
		if mode == netplayServe {
			return "热点没有开起来" + back
		}
		return "没有连上主机的热点 " + netplaySSID + back
	}
	if mode == netplayServe {
		return "热点 " + netplaySSID + " 已开启 · " + netplayHost
	}
	return fmt.Sprintf("已加入主机 · 192.168.80.%d", id)
}

// netplayArgs are RetroArch's arguments for the mode: the host waits for players, a client connects to it
func netplayArgs(mode string) []string {
	nick, _ := os.Hostname()
	switch mode {
	case netplayServe:
		return []string{"--host", "--nick", nick}
	case netplayClient:
		return []string{"--connect", netplayHost, "--nick", nick}
	}
	return nil
}
