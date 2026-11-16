package main

import (
	"os"
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
		h.shows("网络", "↑↓ 分类")
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
		h.press("r", "down", "down", "a")
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
		h.press("r", "down", "down", "a")
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
		h.press("r", "down", "down", "down", "down", "down", "a")
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
		h.until("the new card", func() bool { return h.tt.HasText("New") })
		h.hides("Half")
		if g := h.l.games[h.l.card]; g.id != "beta" {
			t.Errorf("chosen after the rescan: %s, want beta", g.id)
		}
		os.WriteFile(filepath.Join(pack, "dmhost"), nil, 0o755)
		go h.l.refresh(true)
		h.until("the pack's card", func() bool { return h.tt.HasText("Half") })
	})
}
