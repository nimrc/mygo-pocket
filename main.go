// nx2l launcher: the Retroid Pocket Flip 2's home screen on ROCKNIX, in place of EmulationStation. A fullscreen MyGo window in a sway session of
// its own (sway-launcher.conf): the games (nx2linux ports and RetroArch), and the handheld's settings through ROCKNIX's tools (system.go).
// Gamepad: L/R change the page, up/down choose a row, left/right change its value, A does it, B goes back to the games. Touch: a tap does a row,
// − and + step a value (a finger dragging scrolls in MyGo). A game runs as a child process, fullscreen above the launcher; the launcher reads
// no gamepad input while it runs.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/egoist/mygo/ui"
)

type game struct{ id, title, note, cmd string }

var games = []game{
	{"silksong", "空洞骑士：丝之歌", "Switch 原生移植 · nx2linux", "DATA=/storage/nx2linux/silksong /storage/nx2linux/launch.sh"},
	{"hollowknight", "空洞骑士", "Switch 原生移植 · nx2linux", "DATA=/storage/nx2linux/hollowknight /storage/nx2linux/launch.sh"},
	{"retroarch", "RetroArch", "模拟器", "retroarch"},
}

// state is what the pages show, read in the background (system calls are slow; the view never makes one)
type state struct {
	now        time.Time
	bat        battery
	brightness int
	volume     int
	muted      bool
	wifi       wifiState
	nets       []wifiNet
	scanning   bool
	bt         bool
	btDevs     []btDevice
	fan        string
	fanSpeed   int
	cpuTemp    int
	gpuTemp    int
	governor   string
	ssh        bool
	storage    string
	memory     string
	version    string
}

type row struct {
	label, value string
	adjust       func(d int) // left/right and − / +; nil when the row has no value to step
	act          func()      // A and a tap; nil when it does nothing
}

type page struct {
	name string
	rows func(s *state) []row
}

type launcher struct {
	win     *mygo.Window
	mu      sync.Mutex // guards s for the background readers
	s       state
	page    int
	sel     int
	running string // the game playing, "" when none
	busy    string // an action in progress
	note    string // the last action's outcome
	pages   []page
}

func (l *launcher) refresh(full bool) {
	n := state{now: time.Now(), bat: readBattery(), brightness: readBrightness(), wifi: readWifi(), cpuTemp: hottest("cpu"), gpuTemp: hottest("gpu"),
		fanSpeed: fanSpeed(), governor: readGovernor()}
	n.volume, n.muted = readVolume()
	if full {
		n.bt = bluetoothOn()
		if n.bt {
			n.btDevs = readBtDevices()
		}
		n.fan, n.ssh, n.storage, n.memory, n.version = readFanProfile(), sshOn(), storageFree(), memoryFree(), osVersion()
	}
	l.win.Update(func() {
		o := l.s
		if !full {
			n.bt, n.btDevs, n.fan, n.ssh, n.storage, n.memory, n.version = o.bt, o.btDevs, o.fan, o.ssh, o.storage, o.memory, o.version
		}
		n.nets, n.scanning = o.nets, o.scanning
		l.s = n
	})
}

// do runs an action off the main thread, then reads the state again
func (l *launcher) do(what string, f func() string) {
	if l.busy != "" {
		return
	}
	l.busy, l.note = what, ""
	go func() {
		out := f()
		l.refresh(true)
		l.win.Update(func() { l.busy, l.note = "", out })
	}()
}

func onOff(b bool) string {
	if b {
		return "开"
	}
	return "关"
}

func (l *launcher) buildPages() {
	l.pages = []page{
		{"游戏", func(s *state) []row {
			var rs []row
			for _, g := range games {
				rs = append(rs, row{label: g.title, value: g.note, act: func() { l.launch(g) }})
			}
			return rs
		}},
		{"网络", func(s *state) []row {
			rs := []row{
				{label: "Wi-Fi", value: onOff(s.wifi.enabled), act: func() { on := !s.wifi.enabled; l.do("切换 Wi-Fi", func() string { setWifi(on); return "" }) }},
				{label: "当前网络", value: fmt.Sprintf("%s  %s", orDash(s.wifi.ssid), s.wifi.ip)},
				{label: "扫描附近网络", value: map[bool]string{true: "扫描中…", false: fmt.Sprintf("%d 个", len(s.nets))}[s.scanning], act: l.scan},
			}
			for _, n := range s.nets {
				v := fmt.Sprintf("%d%%", n.signal)
				switch {
				case n.inUse:
					v = "已连接 · " + v
				case n.saved:
					v = "已保存 · " + v
				case n.secure:
					v = "加密 · " + v
				}
				rs = append(rs, row{label: "  " + n.ssid, value: v, act: func() { l.do("连接 "+n.ssid, func() string { return firstLine(connectWifi(n)) }) }})
			}
			return rs
		}},
		{"蓝牙", func(s *state) []row {
			rs := []row{{label: "蓝牙", value: onOff(s.bt), act: func() { on := !s.bt; l.do("切换蓝牙", func() string { setBluetooth(on); return "" }) }}}
			for _, d := range s.btDevs {
				v := "已配对"
				if d.connected {
					v = "已连接"
				}
				rs = append(rs, row{label: "  " + d.name, value: v, act: func() { l.do("连接 "+d.name, func() string { toggleBtDevice(d); return "" }) }})
			}
			if s.bt && len(s.btDevs) == 0 {
				rs = append(rs, row{label: "没有已配对的设备", value: "配对新设备请先用 EmulationStation"})
			}
			return rs
		}},
		{"显示与声音", func(s *state) []row {
			return []row{
				{label: "亮度", value: fmt.Sprintf("%d%%", s.brightness), adjust: func(d int) {
					v := clamp(s.brightness+d*5, 5, 100)
					l.s.brightness = v
					go setBrightness(v)
				}},
				{label: "音量", value: fmt.Sprintf("%d%%", s.volume), adjust: func(d int) {
					v := clamp(s.volume+d*5, 0, 100)
					l.s.volume = v
					go setVolume(v)
				}},
				{label: "静音", value: onOff(s.muted), act: func() { l.s.muted = !l.s.muted; go toggleMute() }},
			}
		}},
		{"性能与散热", func(s *state) []row {
			return []row{
				{label: "风扇模式", value: fanNames[s.fan], adjust: func(d int) {
					p := cycle(fanProfiles, s.fan, d)
					l.do("风扇："+fanNames[p], func() string { setFanProfile(p); return "" })
				}},
				{label: "风扇转速", value: fmt.Sprintf("%d%%", s.fanSpeed)},
				{label: "CPU 调频", value: governorNames[s.governor], adjust: func(d int) {
					g := cycle(governors, s.governor, d)
					l.do("调频："+g, func() string { setGovernor(g); return "" })
				}},
				{label: "温度", value: fmt.Sprintf("CPU %d°C · GPU %d°C", s.cpuTemp, s.gpuTemp)},
			}
		}},
		{"系统", func(s *state) []row {
			return []row{
				{label: "SSH", value: onOff(s.ssh), act: func() { on := !s.ssh; l.do("切换 SSH", func() string { setSSH(on); return "" }) }},
				{label: "存储空间", value: s.storage},
				{label: "内存", value: s.memory},
				{label: "系统版本", value: s.version},
				{label: "切换到 EmulationStation", act: func() { l.do("切换到 EmulationStation", func() string { return run(20*time.Second, "/storage/nx2linux/launcher/launcherctl", "es") }) }},
				{label: "重启", act: func() { l.do("重启", func() string { return run(10*time.Second, "systemctl", "reboot") }) }},
				{label: "关机", act: func() { l.do("关机", func() string { return run(10*time.Second, "systemctl", "poweroff") }) }},
			}
		}},
	}
}

func (l *launcher) scan() {
	if l.s.scanning {
		return
	}
	l.s.scanning = true
	go func() {
		nets := scanWifi()
		l.win.Update(func() { l.s.nets, l.s.scanning = nets, false })
	}()
}

func (l *launcher) launch(g game) {
	if l.running != "" {
		return
	}
	l.running, l.note = g.title, ""
	go func() {
		cmd := exec.Command("bash", "-c", g.cmd)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		err := cmd.Run()
		log.Printf("%s ended: %v", g.id, err)
		time.Sleep(500 * time.Millisecond) // the game's last presses are not the launcher's
		l.refresh(true)
		l.win.Update(func() { l.running = "" })
	}()
}

// ---- view ----

var (
	cBg      = ui.Hex("#0b1220")
	cRow     = ui.Hex("#172033")
	cSel     = ui.Hex("#2563eb")
	cText    = ui.Hex("#f1f5f9")
	cMuted   = ui.Hex("#94a3b8")
	cTabIdle = ui.Hex("#1e293b")
)

func (l *launcher) view(c *ui.Context) {
	s := &l.s
	rows := l.pages[l.page].rows(s)
	l.sel = clamp(l.sel, 0, max(0, len(rows)-1))
	ui.Column(c).Fill().Background(cBg).Padding(28).Gap(20).Children(func() {
		// tabs and status
		ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
			for i, p := range l.pages {
				tab := ui.Box(c).PaddingX(18).Padding(10).Radius(10).Background(map[bool]ui.Color{true: cSel, false: cTabIdle}[i == l.page]).Children(func() {
					ui.Text(c, p.name).FontSize(22).TextColor(cText)
				})
				if tab.Clicked() {
					l.page, l.sel = i, 0
				}
			}
			ui.Box(c).Grow(1)
			bat := fmt.Sprintf("电量 %d%%", s.bat.percent)
			if s.bat.charging {
				bat += " 充电中"
			}
			wifi := "Wi-Fi 关"
			if s.wifi.enabled {
				wifi = "Wi-Fi " + orDash(s.wifi.ssid)
			}
			ui.Text(c, fmt.Sprintf("%s    %s    音量 %d%%    %s", wifi, bat, s.volume, s.now.Format("15:04"))).FontSize(20).TextColor(cMuted)
		})
		// rows
		ui.Scroll(c).Grow(1).Gap(10).Children(func() {
			for i, r := range rows {
				el := ui.Row(c).AlignItems(ui.Center).Padding(18).Radius(14).Gap(16).Background(map[bool]ui.Color{true: cSel, false: cRow}[i == l.sel]).Children(func() {
					ui.Text(c, r.label).FontSize(26).TextColor(cText).Grow(1)
					if r.value != "" {
						ui.Text(c, r.value).FontSize(22).TextColor(map[bool]ui.Color{true: cText, false: cMuted}[i == l.sel])
					}
					if r.adjust != nil {
						if ui.Button(c, "  −  ").Clicked() {
							l.sel = i
							r.adjust(-1)
						}
						if ui.Button(c, "  +  ").Clicked() {
							l.sel = i
							r.adjust(1)
						}
					}
				})
				if i == l.sel {
					el.ScrollIntoView()
				}
				if el.Clicked() {
					l.sel = i
					if r.act != nil {
						r.act()
					}
				}
			}
		})
		// footer
		status := "L/R 切换页面    ↑↓ 选择    ←→ 调节    A 确认    B 返回游戏"
		switch {
		case l.running != "":
			status = "正在运行：" + l.running
		case l.busy != "":
			status = l.busy + "…"
		case l.note != "":
			status = l.note
		}
		ui.Text(c, status).FontSize(18).TextColor(cMuted)
	})
}

// ---- gamepad ----

// input polls the pads at 60 Hz; a d-pad direction held repeats after 400 ms, every 120 ms
func (l *launcher) input() {
	var last gamepad.State
	held := map[gamepad.Button]time.Time{}
	repeat := func(s gamepad.State, b gamepad.Button) bool {
		if !s.Pressed(b) {
			delete(held, b)
			return false
		}
		now := time.Now()
		t, ok := held[b]
		switch {
		case !ok:
			held[b] = now.Add(400 * time.Millisecond)
			return true
		case now.After(t):
			held[b] = now.Add(120 * time.Millisecond)
			return true
		}
		return false
	}
	for range time.Tick(16 * time.Millisecond) {
		pads := gamepad.Pads()
		if len(pads) == 0 {
			continue
		}
		var s gamepad.State
		for _, p := range pads { // every pad drives the launcher: the first one is not always the one in the player's hands
			s = merge(s, p.State())
		}
		up, down, left, right := repeat(s, gamepad.Up), repeat(s, gamepad.Down), repeat(s, gamepad.Left), repeat(s, gamepad.Right)
		a, b := s.JustPressed(last, gamepad.East), s.JustPressed(last, gamepad.South)
		lt, rt := s.JustPressed(last, gamepad.LeftShoulder), s.JustPressed(last, gamepad.RightShoulder)
		last = s
		if !(up || down || left || right || a || b || lt || rt) {
			continue
		}
		l.win.Update(func() {
			if l.running != "" {
				return
			}
			rows := l.pages[l.page].rows(&l.s)
			switch {
			case lt || rt:
				l.page = (l.page + map[bool]int{true: -1, false: 1}[lt] + len(l.pages)) % len(l.pages)
				l.sel = 0
			case up && len(rows) > 0:
				l.sel = (l.sel - 1 + len(rows)) % len(rows)
			case down && len(rows) > 0:
				l.sel = (l.sel + 1) % len(rows)
			case (left || right) && l.sel < len(rows) && rows[l.sel].adjust != nil:
				rows[l.sel].adjust(map[bool]int{true: -1, false: 1}[left])
			case a && l.sel < len(rows) && rows[l.sel].act != nil:
				rows[l.sel].act()
			case b:
				l.page, l.sel = 0, 0
			}
		})
	}
}

// merge takes the strongest reading of each control from two pads
func merge(a, b gamepad.State) gamepad.State {
	for i := range a.Buttons {
		a.Buttons[i] = max(a.Buttons[i], b.Buttons[i])
	}
	return a
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

func main() {
	l := &launcher{}
	l.buildPages()
	mygo.App.WhenReady(func() {
		l.win = mygo.NewWindow(mygo.WindowOptions{Title: "nx2l", Frameless: true, Maximized: true, Content: ui.View(l.view)})
		go l.refresh(true)
		go l.input()
		go func() {
			for i := 1; ; i++ {
				time.Sleep(3 * time.Second)
				if l.running == "" {
					l.refresh(i%10 == 0) // the slow readings every 30 s
				}
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
