package main

import (
	"os"
	"strings"
	"path/filepath"
	"testing"
)

// threeGames is a root with a card of each kind that runs here: an nx2linux pack found by its data alone, a native app and a command
func threeGames(t *testing.T) string {
	root := t.TempDir()
	gameDir(t, root, "gamma", map[string]any{"nx2a-data.json": map[string]any{"name": "Gamma"}, "dmhost": ""})
	gameDir(t, root, "beta", map[string]any{"pocket.json": map[string]any{"kind": "native", "title": "Beta", "exec": []string{"run.sh"}, "order": 1}})
	gameDir(t, root, "alpha", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "Alpha", "exec": []string{"true"}, "order": 2}})
	return root
}

func TestGamesScreen(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, threeGames(t))
		h.shows("Gamma", "Beta", "Alpha", "SWITCH · NX2LINUX", "←→ 选择")
		h.shot("games")
		h.press("right")
		h.shows("APP")
		h.hides("SWITCH · NX2LINUX")
		h.press("right", "right")
		if g := h.l.games[h.l.card]; g.id != "gamma" {
			t.Errorf("right past the last card: %s, want gamma", g.id)
		}
		h.press("left")
		h.shows("SYSTEM")
		h.press("r")
		h.shows("网络", pick(h.l.compact, "←→ 分类", "↑↓ 分类"))
		h.press("l")
		h.shows("SYSTEM")
	})
}

func TestNoGames(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, t.TempDir())
		h.shows("没有游戏")
		h.press("right", "a") // nothing to choose or start
		h.press("down")
		h.shows("网络")
	})
}

// TestLaunch starts a game by A: the launcher covers itself and reads no buttons while it runs, and the game runs in its directory with
// its environment
func TestLaunch(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		root := t.TempDir()
		dir := gameDir(t, root, "waits", map[string]any{
			"pocket.json": map[string]any{"kind": "native", "title": "Waits", "exec": []string{"run.sh"}, "env": map[string]string{"FOO": "bar"}},
			"run.sh":      "#!/bin/sh\nwhile [ ! -f go ]; do sleep 0.02; done\n{ pwd; echo \"$GAME_DIR $FOO\"; } > out\n",
		})
		gameDir(t, root, "zz", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "Second", "exec": []string{"true"}, "order": 1}})
		h := newHarness(t, p, nil, root)
		h.press("a")
		h.shows("正在运行")
		h.shot("running")
		h.press("right", "r")
		if h.l.card != 0 || h.l.focus != fGames {
			t.Errorf("presses while the game runs moved the launcher: card %d focus %d", h.l.card, h.l.focus)
		}
		if err := os.WriteFile(filepath.Join(dir, "go"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		h.until("the game ends", func() bool { return h.l.running == "" })
		h.hides("正在运行")
		out, err := os.ReadFile(filepath.Join(dir, "out"))
		if err != nil {
			t.Fatal(err)
		}
		real, _ := filepath.EvalSymlinks(dir)
		want := real + "\n" + dir + " bar\n"
		if got := string(out); got != want && got != dir+"\n"+dir+" bar\n" {
			t.Errorf("the game saw %q, want %q", got, want)
		}
		h.press("right")
		if h.l.card != 1 {
			t.Errorf("after the game: card %d, want 1", h.l.card)
		}
	})
}

// TestTouchLaunch: a tap chooses a card, a tap on the chosen one starts it
func TestTouchLaunch(t *testing.T) {
	onPanels(t, true, func(t *testing.T, p panel) {
		root := t.TempDir()
		gameDir(t, root, "a", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "First", "exec": []string{"true"}}})
		dir := gameDir(t, root, "b", map[string]any{
			"pocket.json": map[string]any{"kind": "native", "title": "Tapped", "exec": []string{"run.sh"}},
			"run.sh":      "#!/bin/sh\ntouch started\n",
		})
		h := newHarness(t, p, nil, root)
		h.click("Tapped")
		if h.l.card != 1 || h.l.running != "" {
			t.Fatalf("first tap: card %d running %q, want card 1 and nothing running", h.l.card, h.l.running)
		}
		h.click("Tapped")
		h.until("the game ends", func() bool { return h.l.running == "" && exists(filepath.Join(dir, "started")) })
		h.click("设置")
		h.shows("↑↓ 分类")
		h.click("显示")
		h.shows("界面缩放")
		h.click("界面缩放")
		h.click("›")
		if h.l.zoom != 1.75 {
			t.Errorf("tapping › made the zoom %g, want 1.75", h.l.zoom)
		}
		h.click("游戏")
		h.shows("←→ 选择")
	})
}

// TestValueNeedsA: in a row of settings left is back; a value changes only after A, and A or B is done with it
func TestValueNeedsA(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, threeGames(t))
		h.toCategory(2)
		h.press("a")
		h.shows("显示", "界面缩放", "1.5x", "A 修改")
		h.press("down", "left")
		if h.l.focus != fCategories || h.l.zoom != 1.5 {
			t.Fatalf("left on a value: focus %d zoom %g, want the categories and 1.5", h.l.focus, h.l.zoom)
		}
		h.press("a", "down", "a")
		h.shows("←→ 调节    A / B 完成")
		h.shot("editing")
		h.press("right")
		h.shows("1.75x")
		h.until("the zoom is kept", func() bool { return sys.ran(`set_setting mygo-pocket.ui.zoom "1.75"`) })
		h.press("up") // still editing: up does nothing
		h.press("b")
		if h.l.editing || h.l.focus != fRows {
			t.Errorf("B after a change: editing %v focus %d, want the rows", h.l.editing, h.l.focus)
		}
		h.press("left")
		if h.l.zoom != 1.75 || h.l.focus != fCategories {
			t.Errorf("left after editing: zoom %g focus %d", h.l.zoom, h.l.focus)
		}
	})
}

func TestStickLights(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, threeGames(t))
		h.l.s.ledMode, h.l.s.ledBright, h.l.s.ledColor = "battery", 100, -1
		h.toCategory(2)
		h.press("a")
		h.shows("摇杆灯", "电量")
		h.hides("颜色")
		h.press("down", "down", "down") // over the section title
		if r := categories[h.l.cat].rows(h.l)[h.l.sel]; r.key != "led-brightness" {
			t.Fatalf("on %q, want the lights' brightness", r.label)
		}
		h.press("a", "left", "left", "left", "a")
		if h.l.s.ledBright != 85 || ledLevel.Load() != 85 {
			t.Errorf("brightness %d level %d, want 85", h.l.s.ledBright, ledLevel.Load())
		}
		h.until("the brightness is kept", func() bool { return sys.ran(`set_setting mygo-pocket.led.brightness "85"`) })
		h.press("up", "a", "right", "a")
		h.shows("颜色", "白")
		h.until("rgb mode", func() bool { return sys.ran("ledcontrol rgb") })
		h.shot("lights")
		h.press("down", "a", "right", "right")
		h.shows("橙")
		h.until("orange", func() bool { return sys.ran(`set_setting analogsticks.led "217 255 80 0 255 80 0"`) })
	})
}

// TestRebootAsksFirst: a power action waits for a confirmation, which B cancels
func TestRebootAsksFirst(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, threeGames(t))
		h.toCategory(5)
		h.press("a")
		h.shows("电源", "重启", "关机")
		h.l.find("电源", "重启") // the rows above it are the battery's, as many as the gauge reports
		h.tt.Frame()
		h.press("a")
		h.shows("重启掌机？", "取消", "确认")
		h.shot("confirm")
		h.press("right", "b")
		h.hides("重启掌机？")
		h.press("a", "a") // cancel is chosen first
		h.press("a", "right", "a")
		h.until("reboot", func() bool { return sys.ran("systemctl reboot") })
		if n := sys.count("systemctl reboot"); n != 1 {
			t.Errorf("reboot ran %d times", n)
		}
	})
}

// TestJoinWifi: a secured network asks for its password on the keyboard, then joins with it
func TestJoinWifi(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, map[string]string{"nmcli dev wifi connect": "Device 'wlan0' successfully activated"}, threeGames(t))
		h.l.s.wifi.enabled = true
		h.l.s.nets = []wifiNet{{ssid: "Home", secure: true, signal: 80}}
		h.l.find("网络", "Home")
		h.tt.Frame()
		h.press("a")
		h.shows("输入 Home 的密码")
		h.press("a", "right", "a", "x", "a", "b") // q, w, shift W, erase it
		h.l.modal.(*keyboard).text += "-pw"
		h.tt.Frame()
		h.shot("keyboard")
		h.press("start")
		h.until("joined", func() bool { return h.tt.HasText("已连接 Home") })
		if !sys.ran("nmcli dev wifi connect Home password qw-pw") {
			t.Errorf("commands %q", sys.cmds)
		}
	})
}

// TestControl drives the launcher through MYGO_POCKET_CTL, as the device tests do over SSH
func TestControl(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, threeGames(t))
		path := filepath.Join(t.TempDir(), "ctl")
		go h.l.control(path)
		h.until("the FIFO", func() bool { st, err := os.Stat(path); return err == nil && st.Mode()&os.ModeNamedPipe != 0 })
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString("find 显示 界面缩放\na\nright\nb\n")
		f.Close()
		h.until("1.75x", func() bool { return h.l.zoom == 1.75 && !h.l.editing })
		f, _ = os.OpenFile(path, os.O_WRONLY, 0)
		f.WriteString("game alpha\nnonsense\n")
		f.Close()
		h.until("alpha chosen", func() bool { return h.l.focus == fGames && h.l.games[h.l.card].id == "alpha" })
		h.shows("SYSTEM")
	})
}

// TestRescan: a directory copied in is a card at the next full refresh, an nx2linux pack once its dmhost is in, and the chosen card stays
func TestRescan(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		root := threeGames(t)
		h := newHarness(t, p, nil, root)
		h.press("right") // beta
		gameDir(t, root, "aaa", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "New", "exec": []string{"true"}}})
		pack := gameDir(t, root, "copying", map[string]any{"nx2a-data.json": map[string]any{"name": "Half"}})
		go h.l.refresh(true)
		h.until("the new card", func() bool { return h.hasCard("aaa") })
		if h.hasCard("copying") {
			t.Error("the pack is a card before its dmhost")
		}
		if g := h.l.games[h.l.card]; g.id != "beta" {
			t.Errorf("chosen after the rescan: %s, want beta", g.id)
		}
		os.WriteFile(filepath.Join(pack, "dmhost"), nil, 0o755)
		go h.l.refresh(true)
		h.until("the pack's card", func() bool { return h.hasCard("copying") })
	})
}

// TestCompact: on the RG ARC's panel the cards show three at a time around the choice, and the settings' categories are a row of icons
// that left and right go along
func TestCompact(t *testing.T) {
	root := threeGames(t)
	for _, id := range []string{"delta", "epsilon"} {
		gameDir(t, root, id, map[string]any{"pocket.json": map[string]any{"kind": "command", "title": strings.ToUpper(id[:1]) + id[1:], "exec": []string{"true"}, "order": 3}})
	}
	h := newHarness(t, panels[1], nil, root)
	if !h.l.compact {
		t.Fatal("the 640x480 panel is not compact")
	}
	h.shows("Gamma", "Beta", "›")
	h.hides("Delta", "Epsilon")
	h.press("right", "right", "right")
	h.shows("Alpha", "Delta", "Epsilon")
	h.hides("Gamma")
	h.press("r")
	h.shows("网络", "Wi-Fi", "←→ 分类")
	h.press("right")
	if h.l.cat != 1 || h.l.focus != fCategories {
		t.Fatalf("right on the categories: category %d focus %d, want 蓝牙's", h.l.cat, h.l.focus)
	}
	h.press("up") // nothing above the icons
	h.press("left", "left")
	if h.l.cat != len(categories)-1 {
		t.Errorf("left past the first category: %d, want the last", h.l.cat)
	}
	h.shows("系统")
	h.shot("settings")
	h.press("down")
	if h.l.focus != fRows {
		t.Errorf("down on the categories: focus %d, want the rows", h.l.focus)
	}
}

// TestSeriesScreen: A opens a series into a row of its own, its games start from there, and B goes back out onto the series' card; the
// FIFO's "game <id>" opens the series a game is in
func TestSeriesScreen(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		root := t.TempDir()
		gameDir(t, root, "solo", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "Solo", "exec": []string{"true"}}})
		series := gameDir(t, root, "zz-mslug", map[string]any{"pocket.json": map[string]any{"kind": "series", "title": "合金弹头", "subtitle": "Metal Slug"}})
		gameDir(t, series, "ms1", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "MS 1", "exec": []string{"true"}, "order": 1}})
		dir := gameDir(t, series, "ms2", map[string]any{
			"pocket.json": map[string]any{"kind": "native", "title": "MS 2", "exec": []string{"run.sh"}, "order": 2},
			"run.sh":      "#!/bin/sh\ntouch started\n",
		})
		h := newHarness(t, p, nil, root)
		h.press("right")
		h.shows("合金弹头", "SERIES · 2", "A 打开")
		h.press("a")
		if h.l.series != "zz-mslug" || h.l.card != 0 {
			t.Fatalf("A on the series: series %q card %d", h.l.series, h.l.card)
		}
		h.shows("MS 1", "‹  合金弹头", "B 返回")
		h.hides("Solo")
		h.shot("series")
		h.press("right", "a")
		h.until("MS 2 ends", func() bool { return h.l.running == "" && exists(filepath.Join(dir, "started")) })
		if h.l.series != "zz-mslug" || h.l.card != 1 {
			t.Errorf("after the game: series %q card %d, want the series' second card", h.l.series, h.l.card)
		}
		h.press("b")
		if h.l.series != "" || h.l.games[h.l.card].id != "zz-mslug" {
			t.Errorf("B in the series: series %q on %s, want the series' card", h.l.series, h.l.games[h.l.card].id)
		}
		h.shows("Solo", "A 打开")
		h.press("b") // nothing to go back out of
		if h.l.focus != fGames || h.l.games[h.l.card].id != "zz-mslug" {
			t.Errorf("B on the cards moved the launcher")
		}
		h.l.chooseID("ms2")
		h.tt.Frame()
		h.shows("MS 2", "‹  合金弹头")
		os.RemoveAll(series) // the open series is gone at the next scan: back on the cards
		go h.l.refresh(true)
		h.until("the series gone", func() bool { return !h.hasCard("zz-mslug") })
		if h.l.series != "" {
			t.Errorf("the series is gone but still open")
		}
		h.shows("Solo")
	})
}

// TestTouchSeries: a tap on the chosen series opens it, a tap on its name goes back out
func TestTouchSeries(t *testing.T) {
	onPanels(t, true, func(t *testing.T, p panel) {
		root := t.TempDir()
		series := gameDir(t, root, "pk", map[string]any{"pocket.json": map[string]any{"kind": "series", "title": "宝可梦"}})
		gameDir(t, series, "gold", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "Gold", "exec": []string{"true"}}})
		h := newHarness(t, p, nil, root)
		h.click("宝可梦")
		h.shows("Gold")
		h.click("‹  宝可梦")
		h.hides("Gold")
		if h.l.series != "" {
			t.Errorf("a tap on the series' name left it open")
		}
	})
}

// TestManyCards: on a large panel the cards keep their size, four across the Flip 2 around the choice
func TestManyCards(t *testing.T) {
	root := t.TempDir()
	for i, id := range []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7"} {
		gameDir(t, root, id, map[string]any{"pocket.json": map[string]any{"kind": "command", "title": strings.ToUpper(id), "exec": []string{"true"}, "order": i}})
	}
	h := newHarness(t, panels[0], nil, root)
	h.shows("C1", "C4", "›")
	h.hides("C5", "C6", "C7")
	h.press("right", "right", "right", "right", "right")
	h.shows("C4", "C7")
	h.hides("C1", "C3")
}

// TestLocalNetplay: 网络 → 本地联机 opens ROCKNIX's ad-hoc access point as the host, and a RetroArch card then starts RetroArch hosting;
// as a client it connects to the host's address
func TestLocalNetplay(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		bin, cores, out := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "args")
		os.WriteFile(filepath.Join(bin, "retroarch"), []byte("#!/bin/sh\necho \"$@\" > "+out+"\n"), 0o755)
		os.WriteFile(filepath.Join(cores, "snes9x_libretro.so"), nil, 0o644)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("RETROARCH_CORES", cores)
		root := t.TempDir()
		gameDir(t, root, "snes", map[string]any{"pocket.json": map[string]any{"kind": "retroarch", "title": "SNES", "core": "snes9x", "rom": "g.sfc"}})
		h := newHarness(t, p, map[string]string{"dev show wlan0": "GENERAL.CONNECTION:NETPLAY_AP\nIP4.ADDRESS[1]:192.168.80.1/24",
			"get_setting wifi.adhoc": "1"}, root) // the refresh after the change reads the mode back
		h.l.s.netplay = netplayOff
		h.l.find("网络", "模式")
		h.tt.Frame()
		h.shows("本地联机", "模式", "关闭")
		h.press("a", "right", "b")
		h.until("the access point", func() bool { return h.tt.HasText("热点 NETPLAY_AP 已开启") })
		h.shot("host")
		for _, c := range []string{`set_setting wifi.adhoc.enabled "1"`, `set_setting wifi.adhoc.id "1"`, "wifictl connect"} {
			if !sys.ran(c) {
				t.Errorf("%q not run; commands %q", c, sys.cmds)
			}
		}
		h.press("l")
		play := func() string {
			os.Remove(out)
			h.press("a")
			h.until("RetroArch ran", func() bool { return h.l.running == "" && exists(out) })
			b, _ := os.ReadFile(out)
			return strings.TrimSpace(string(b))
		}
		if a := play(); !strings.Contains(a, "g.sfc --host --nick") {
			t.Errorf("host's RetroArch: %q", a)
		}
		h.l.s.netplay = netplayClient
		if a := play(); !strings.Contains(a, "g.sfc --connect 192.168.80.1 --nick") {
			t.Errorf("client's RetroArch: %q", a)
		}
		h.l.s.netplay = netplayOff
		if a := play(); !strings.HasSuffix(a, "g.sfc") {
			t.Errorf("RetroArch alone: %q", a)
		}
	})
}
