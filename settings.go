package main

import (
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"
	"time"
)

// state is what the screens show, read in the background: system calls are slow and the view never makes one
type state struct {
	now                    time.Time
	bat                    battery
	batInfo                batteryInfo
	brightness, volume     int
	muted                  bool
	sinks                  []sink // PipeWire picks the headphones while they are plugged in (UCM), the speaker otherwise
	wifi                   wifiState
	signal                 int // the network in use, from the last scan
	nets                   []wifiNet
	wifiScanning           bool
	netplay                string // netplayOff, netplayServe or netplayClient
	bt                     bool
	btDevs, btFound        []btDevice
	btScanning             bool
	fan                    string
	fanSpeed               int
	cpuTemp, gpuTemp       int
	cpuFreq                string
	governor, gpuGovernor  string
	gpuFreq                int
	ssh, samba, syncthing  bool
	timesync               bool
	ledMode                string // "" when the handheld has no stick lights
	ledBright, ledColor    int
	timezone               string
	storage, memory        string
	version, kernel, uptime string
}

func (l *launcher) refresh(full bool) {
	n := state{now: time.Now(), bat: readBattery(), batInfo: readBatteryInfo(), brightness: readBrightness(), wifi: readWifi(),
		cpuTemp: hottest("cpu"), gpuTemp: hottest("gpu"), fanSpeed: fanSpeed(), governor: readGovernor(), gpuGovernor: readGPUGovernor(),
		cpuFreq: cpuFreqs(), gpuFreq: gpuFreq(), memory: memoryFree(), uptime: uptime()}
	n.volume, n.muted = readVolume()
	// the switches every refresh (systemctl is-active takes milliseconds): a switch shown from a stale reading flips the wrong way
	n.bt, n.ssh, n.samba, n.syncthing, n.timesync = bluetoothOn(), sshOn(), serviceOn("smbd"), serviceOn("syncthing"), serviceOn("systemd-timesyncd")
	if full {
		if n.bt {
			n.btDevs = readBtDevices()
		}
		n.sinks = readSinks()
		n.nets = markInUse(cachedWifi(), n.wifi.ssid)
		n.fan = readFanProfile()
		n.timezone, n.storage, n.version, n.kernel = readTimezone(), storageFree(), osVersion(), kernel()
		n.netplay = readNetplay()
		n.ledMode, n.ledBright, n.ledColor = readLEDMode(), readLEDBrightness(), readLEDColor()
	}
	var gs []game
	if full {
		gs = scanGames()
	}
	back, rejoin := keepProvisioning(n.wifi.ssid) // every refresh: iwd loses an 802.1X network's file when it restarts
	if rejoin != "" {
		go func() { log.Printf("rejoin %s: %s", rejoin, firstLine(rejoinWifi(rejoin))) }()
	}
	l.update(func() {
		if full {
			l.setGames(gs)
		}
		o := l.s
		if !full {
			n.btDevs, n.sinks, n.fan = o.btDevs, o.sinks, o.fan
			n.timezone, n.storage, n.version, n.kernel, n.netplay = o.timezone, o.storage, o.version, o.kernel, o.netplay
			n.ledMode, n.ledBright, n.ledColor = o.ledMode, o.ledBright, o.ledColor
		}
		if !full || o.wifiScanning {
			n.nets = o.nets
		}
		n.wifiScanning, n.btFound, n.btScanning, n.signal = o.wifiScanning, o.btFound, o.btScanning, o.signal
		for _, w := range n.nets {
			if w.inUse {
				n.signal = w.signal
			}
		}
		l.s = n
		if len(back) > 0 {
			l.say("已恢复 " + strings.Join(back, "、") + " 的登录配置" + pick(rejoin != "", "，正在重新连接", ""))
		}
	})
}

// ---- rows ----

type rowKind int

const (
	kInfo   rowKind = iota // a value to read
	kToggle                // on or off
	kChoice                // one of a few values, ‹ › to step
	kSlider                // a percentage
	kAction                // does something
	kHeader                // a section title in the list
)

type row struct {
	kind         rowKind
	key          string // what the selection follows when rows move (a device's address); the label when empty
	label, value string
	on           bool
	detail       string // a second line under the label
	adjust       func(d int)
	act          func()
	alt          func() // X
	altLabel     string
	slider       *float64
	set          func(v int)
	danger       bool
}

func (r row) id() string {
	if r.key != "" {
		return r.key
	}
	return r.label
}

type category struct {
	name, icon string
	rows       func(l *launcher) []row
}

// toggle flips *on in the state at once, so the switch shows it and a second press reads it, and applies it in the background
func toggle(label string, on *bool, set func(bool), l *launcher, what string) row {
	return row{kind: kToggle, label: label, on: *on, act: func() {
		if l.busy != "" {
			l.say("请稍候：" + l.busy)
			return
		}
		want := !*on
		*on = want
		l.do(what, func() string { set(want); return "" })
	}}
}

func choice(label, value string, adjust func(d int)) row {
	return row{kind: kChoice, label: label, value: value, adjust: adjust}
}

func info(label, value string) row { return row{kind: kInfo, label: label, value: value} }

func header(label string) row { return row{kind: kHeader, label: label} }

var categories = []category{
	{"网络", "◉", func(l *launcher) []row {
		s := &l.s
		rs := []row{toggle("Wi-Fi", &s.wifi.enabled, setWifi, l, "切换 Wi-Fi")}
		if s.wifi.enabled {
			cur := "未连接"
			if s.wifi.ssid != "" {
				cur = s.wifi.ssid + "  ·  " + cmp(s.wifi.ip, "正在获取地址…")
			}
			rs = append(rs, info("当前网络", cur))
			scan := fmt.Sprintf("找到 %d 个", len(s.nets))
			if s.wifiScanning {
				scan = "扫描中…"
			}
			rs = append(rs, row{kind: kAction, label: "扫描附近网络", value: scan, act: l.scanWifi})
			for _, n := range s.nets {
				state := fmt.Sprintf("信号 %d%%", n.signal)
				switch {
				case n.inUse:
					state = "已连接 · " + state
				case n.saved:
					state = "已保存 · " + state
				case n.enterprise:
					state = "企业 · " + state
				case n.secure:
					state = "加密 · " + state
				default:
					state = "开放 · " + state
				}
				r := row{kind: kAction, label: n.ssid, value: state, act: func() { l.joinWifi(n) }}
				if n.saved {
					r.alt, r.altLabel = func() { l.confirm("忘记 "+n.ssid+"？", func() string { return forgetWifi(n.ssid) }) }, "忘记"
				}
				rs = append(rs, r)
			}
		}
		mode := netplayNames[s.netplay]
		if s.netplay != netplayOff && s.wifi.ssid == netplaySSID {
			mode += " · " + s.wifi.ip
		}
		rs = append(rs, header("本地联机（无网络时）"), row{kind: kAction, label: "模式", value: mode, act: l.pickNetplay})
		rs = append(rs, header("网络服务"),
			toggle("SSH 远程登录", &s.ssh, setSSH, l, "切换 SSH"),
			toggle("Samba 文件共享", &s.samba, func(on bool) { setService("smbd", "samba.enabled", on) }, l, "切换 Samba"),
			toggle("Syncthing 同步", &s.syncthing, func(on bool) { setService("syncthing", "syncthing.enabled", on) }, l, "切换 Syncthing"))
		return rs
	}},
	{"蓝牙", "◈", func(l *launcher) []row {
		s := &l.s
		rs := []row{toggle("蓝牙", &s.bt, setBluetooth, l, "切换蓝牙")}
		if !s.bt {
			return rs
		}
		rs = append(rs, header("已配对的设备"))
		for _, d := range s.btDevs {
			st := "未连接"
			if d.connected {
				st = "已连接"
			}
			rs = append(rs, row{kind: kAction, key: d.mac, label: d.name, value: st, act: func() { l.do("连接 "+d.name, func() string { return toggleBtDevice(d) }) },
				alt: func() { l.do("忘记 "+d.name, func() string { return forgetBluetooth(d) }) }, altLabel: "忘记"})
		}
		if len(s.btDevs) == 0 {
			rs = append(rs, info("没有已配对的设备", ""))
		}
		scan := "找新设备"
		if s.btScanning {
			scan = "扫描中…"
		}
		rs = append(rs, header("附近的设备"), row{kind: kAction, label: "扫描", value: scan, act: l.scanBluetooth})
		for _, d := range s.btFound {
			rs = append(rs, row{kind: kAction, key: d.mac, label: d.name, value: "配对", act: func() { l.do("配对 "+d.name, func() string { return pairBluetooth(d) }) }})
		}
		return rs
	}},
	{"显示", "☀", func(l *launcher) []row {
		s := &l.s
		rs := []row{
			{kind: kSlider, label: "亮度", value: fmt.Sprintf("%d%%", s.brightness), slider: &l.slideBrightness,
				adjust: func(d int) { l.setBrightness(s.brightness + d*5) }, set: l.setBrightness},
			choice("界面缩放", fmt.Sprintf("%gx", l.zoom), func(d int) {
				i := 0
				for j, z := range zooms {
					if z == l.zoom {
						i = j
					}
				}
				l.zoom = zooms[clamp(i+d, 0, len(zooms)-1)]
				go setSetting("mygo-pocket.ui.zoom", fmt.Sprint(l.zoom))
			}),
		}
		if s.ledMode == "" {
			return rs
		}
		mode := 0
		for i, m := range ledModes {
			if m.id == s.ledMode {
				mode = i
			}
		}
		rs = append(rs, header("摇杆灯"), choice("模式", ledModes[mode].name, func(d int) {
			s.ledMode = ledModes[clamp(mode+d, 0, len(ledModes)-1)].id
			if s.ledMode == "rgb" && s.ledColor < 0 {
				s.ledColor = 0
			}
			m, c, b := s.ledMode, s.ledColor, s.ledBright
			go func() {
				if m == "rgb" {
					setLEDColor(c, b)
				} else {
					setLEDMode(m)
				}
			}()
		}))
		if s.ledMode == "off" {
			return rs
		}
		if s.ledMode == "rgb" {
			name := "自定义"
			if s.ledColor >= 0 {
				name = ledColors[s.ledColor].name
			}
			rs = append(rs, choice("颜色", name, func(d int) {
				s.ledColor = clamp(s.ledColor+d, 0, len(ledColors)-1)
				c, b := s.ledColor, s.ledBright
				go setLEDColor(c, b)
			}))
		}
		rs = append(rs, row{kind: kSlider, label: "亮度", key: "led-brightness", value: fmt.Sprintf("%d%%", s.ledBright), slider: &l.slideLED,
			adjust: func(d int) { l.setLEDBrightness(s.ledBright + d*5) }, set: l.setLEDBrightness})
		return rs
	}},
	{"声音", "♪", func(l *launcher) []row {
		s := &l.s
		rs := []row{
			{kind: kSlider, label: "音量", value: fmt.Sprintf("%d%%", s.volume), slider: &l.slideVolume,
				adjust: func(d int) { l.setVolume(s.volume + d*5) }, set: l.setVolume},
			{kind: kToggle, label: "静音", on: s.muted, act: func() { l.s.muted = !l.s.muted; go toggleMute() }},
		}
		out := "扬声器"
		for _, k := range s.sinks {
			if k.def {
				out = sinkName(k)
			}
		}
		rs = append(rs, info("输出设备", out+"（插拔耳机自动切换）"))
		return rs
	}},
	{"性能", "⚡", func(l *launcher) []row {
		s := &l.s
		// fanSpeed is -1 without a pwmfan: the fan rows are left out on a handheld without one, such as the RG ARC
		fan := s.fanSpeed >= 0
		rs := []row{
			choice("CPU 调频", governorNames[s.governor], func(d int) {
				g := cycle(governors, s.governor, d)
				l.do("CPU 调频："+governorNames[g], func() string { setGovernor(g); return "" })
			}),
			choice("GPU 调频", gpuGovernorNames[s.gpuGovernor], func(d int) {
				g := cycle(gpuGovernors, s.gpuGovernor, d)
				l.do("GPU 调频："+gpuGovernorNames[g], func() string { setGPUGovernor(g); return "" })
			}),
		}
		if fan {
			rs = append(rs, choice("风扇模式", fanNames[s.fan], func(d int) {
				p := cycle(fanProfiles, s.fan, d)
				l.do("风扇："+fanNames[p], func() string { setFanProfile(p); return "" })
			}))
		}
		rs = append(rs, header("实时状态"),
			info("CPU", fmt.Sprintf("%d°C  ·  %s", s.cpuTemp, s.cpuFreq)),
			info("GPU", fmt.Sprintf("%d°C  ·  %d MHz", s.gpuTemp, s.gpuFreq)))
		if fan {
			rs = append(rs, info("风扇转速", fmt.Sprintf("%d%%", s.fanSpeed)))
		}
		return rs
	}},
	{"电源", "▮", func(l *launcher) []row {
		s := &l.s
		st := "放电中"
		if s.bat.charging {
			st = "充电中"
		}
		b := s.batInfo
		rs := []row{info("电量", fmt.Sprintf("%d%%  ·  %s", s.bat.percent, st))}
		if b.voltage > 0 {
			rs = append(rs, info("功率", fmt.Sprintf("%.1f W  ·  %.2f V  ·  %.2f A", abs(b.voltage*b.current), b.voltage, abs(b.current))))
		}
		if b.hasTemp {
			rs = append(rs, info("电池温度", strings.TrimSuffix(fmt.Sprintf("%.1f°C  ·  %s", b.temp, b.health), "  ·  ")))
		}
		return append(rs,
			header("电源"),
			choice("自动待机", standbyName(int(l.standbyMin.Load())), l.setStandby),
			row{kind: kAction, label: "重启", act: func() { l.confirm("重启掌机？", func() string { return run(10*time.Second, "systemctl", "reboot") }) }},
			row{kind: kAction, label: "关机", danger: true, act: func() { l.confirm("关机？", func() string { return run(10*time.Second, "systemctl", "poweroff") }) }},
		)
	}},
	{"系统", "⚙", func(l *launcher) []row {
		s := &l.s
		return []row{
			choice("时区", s.timezone, func(d int) {
				tz := cycle(timezones, s.timezone, d)
				l.do("时区："+tz, func() string { return setTimezone(tz) })
			}),
			toggle("网络时间同步", &s.timesync, func(on bool) { setService("systemd-timesyncd", "", on) }, l, "切换时间同步"),
			header("关于"),
			info("IP 地址", orDash(s.wifi.ip)),
			info("存储空间", s.storage+" 可用"),
			info("内存", s.memory),
			info("运行时间", s.uptime),
			info("系统", s.version),
			info("内核", s.kernel),
		}
	}},
}

func (l *launcher) setBrightness(v int) {
	v = clamp(v, 5, 100)
	l.s.brightness = v
	go setBrightness(v)
}

func (l *launcher) setLEDBrightness(v int) {
	v = clamp(v, 5, 100)
	l.s.ledBright = v
	ledLevel.Store(int32(v))
	go holdLEDBrightness(v)
	go setSetting("mygo-pocket.led.brightness", strconv.Itoa(v))
}

func (l *launcher) setVolume(v int) {
	v = clamp(v, 0, 100)
	l.s.volume = v
	go setVolume(v)
}

func (l *launcher) scanWifi() {
	if l.s.wifiScanning {
		return
	}
	l.s.wifiScanning = true
	go func() {
		nets := scanWifi()
		l.update(func() { l.s.nets, l.s.wifiScanning = nets, false })
	}()
}

func (l *launcher) scanBluetooth() {
	if l.s.btScanning {
		return
	}
	l.s.btScanning = true
	go func() {
		found := scanBluetooth()
		l.update(func() { l.s.btFound, l.s.btScanning = found, false })
	}()
}

// joinWifi connects a saved or open network at once, and asks a secured new one for its password
func (l *launcher) joinWifi(n wifiNet) {
	if n.inUse {
		return
	}
	if n.saved || !n.secure {
		l.do("连接 "+n.ssid, func() string { return connectWifi(n) })
		return
	}
	if n.enterprise {
		user := newKeyboard(n.ssid+" 的账号", func(id string) {
			pw := newKeyboard(n.ssid+" 的密码（"+id+"）", func(pw string) {
				l.do("连接 "+n.ssid, func() string { return joinEnterprise(n.ssid, id, pw) })
			})
			pw.check = func(t string) string { return pick(t == "", "请输入密码", "") }
			l.modal = pw
		})
		user.plain = true
		user.check = func(t string) string { return pick(strings.TrimSpace(t) == "", "请输入账号，例如邮箱", "") }
		l.modal = user
		return
	}
	k := newKeyboard("输入 "+n.ssid+" 的密码", func(pw string) {
		l.do("连接 "+n.ssid, func() string { return joinWifi(n.ssid, pw) })
	})
	k.check = func(t string) string { return pick(len(t) < 8, "WPA 密码至少 8 位", "") }
	l.modal = k
}

// pickNetplay asks which local netplay mode to switch to; nothing changes until one is taken
func (l *launcher) pickNetplay() {
	if l.busy != "" {
		l.say("请稍候：" + l.busy)
		return
	}
	cur := slices.Index(netplayModes, l.s.netplay)
	l.modal = &pickBox{title: "本地联机", cur: max(cur, 0), sel: max(cur, 0), options: []pickOption{
		{netplayNames[netplayOff], "回到保存过的 Wi-Fi"},
		{netplayNames[netplayServe], "开热点 " + netplaySSID + "，其他掌机加入；RetroArch 游戏等玩家连进来"},
		{netplayNames[netplayClient], "连上主机的热点；RetroArch 游戏自动连到主机"},
	}, take: func(i int) {
		m := netplayModes[i]
		l.s.netplay = m
		l.do("本地联机："+netplayNames[m], func() string { return setNetplay(m) })
	}}
}

func (l *launcher) confirm(question string, f func() string) {
	l.modal = &confirmBox{question: question, yes: func() { l.do(question, f) }}
}

func sinkName(k sink) string {
	switch {
	case strings.Contains(strings.ToLower(k.name), "headphone"):
		return "耳机"
	case strings.Contains(strings.ToLower(k.name), "speaker"):
		return "扬声器"
	}
	return k.name
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func cycle(list []string, cur string, d int) string {
	i := 0
	for j, v := range list {
		if v == cur {
			i = j
		}
	}
	return list[(i+d+len(list))%len(list)]
}

func orDash(s string) string {
	if s == "" {
		return "未连接"
	}
	return s
}

func firstLine(s string) string { f, _, _ := strings.Cut(s, "\n"); return f }

// zooms are the view's zooms 界面缩放 steps through; 1.5 by default: at 1 the 1080p panel's 5.5 inches make text too small
var zooms = []float32{1, 1.25, 1.5, 1.75, 2}

func readZoom() float32 {
	var z float32
	if _, err := fmt.Sscan(getSetting("mygo-pocket.ui.zoom"), &z); err != nil || z < 1 || z > 2 {
		return 1.5
	}
	return z
}
