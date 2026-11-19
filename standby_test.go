package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/game/gamepad"
)

// TestStandbyRow: 电源 → 自动待机 shows the minutes and steps through the choices, keeping them in ROCKNIX's settings
func TestStandbyRow(t *testing.T) {
	onPanels(t, false, func(t *testing.T, p panel) {
		h := newHarness(t, p, nil, threeGames(t))
		h.l.standbyMin.Store(10)
		h.l.find("电源", "自动待机")
		h.tt.Frame()
		h.shows("自动待机", "10 分钟")
		h.press("a", "right", "b")
		h.shows("30 分钟")
		h.until("kept", func() bool { return sys.ran(`set_setting mygo-pocket.standby "30"`) })
		h.press("a", "left", "left", "left", "left", "b")
		h.shows("关闭")
	})
}

// TestAutoStandby: idle on the launcher, a warning comes first and any button cancels it; left idle, the handheld goes to standby once;
// a game running is never put to standby
func TestAutoStandby(t *testing.T) {
	unit, warn, sb, flag0 := standbyUnit, standbyWarn, standby, fakeSuspended
	t.Cleanup(func() { standbyUnit, standbyWarn, standby, fakeSuspended = unit, warn, sb, flag0 })
	standbyUnit, standbyWarn = 400*time.Millisecond, 200*time.Millisecond
	var slept atomic.Int32
	standby = func() { slept.Add(1) }

	h := newHarness(t, panels[1], nil, threeGames(t))
	h.l.standbyMin.Store(1) // 400 ms here
	h.l.touch()
	check := func() { h.l.checkStandby(); h.drain(); h.tt.Frame() }

	time.Sleep(250 * time.Millisecond)
	check()
	h.shows("即将待机", "按任意键取消")
	h.shot("warning")
	h.press("down") // any button
	if h.l.modal != nil {
		t.Fatal("a button did not cancel the warning")
	}
	time.Sleep(250 * time.Millisecond)
	check() // the idle time started again at the press: a warning, no standby yet
	if slept.Load() != 0 {
		t.Fatal("standby before the time was up")
	}
	time.Sleep(200 * time.Millisecond)
	check()
	if slept.Load() != 1 || h.l.modal != nil {
		t.Fatalf("standby %d times, modal %v", slept.Load(), h.l.modal)
	}

	h.l.playing.Store(true)
	time.Sleep(500 * time.Millisecond)
	check()
	h.l.playing.Store(false)
	check()
	if slept.Load() != 1 {
		t.Errorf("standby while a game ran: %d", slept.Load())
	}

	flag := filepath.Join(t.TempDir(), "fake-suspend.flag") // in a fake suspend, a second toggle would wake the handheld
	fakeSuspended = flag
	os.WriteFile(flag, nil, 0o644)
	time.Sleep(500 * time.Millisecond)
	check()
	if slept.Load() != 1 {
		t.Errorf("standby again during a fake suspend: %d", slept.Load())
	}
}

func TestNewPress(t *testing.T) {
	var rest, down, downA, trig gamepad.State
	down.Buttons[gamepad.Down] = 1
	downA.Buttons[gamepad.Down], downA.Buttons[gamepad.East] = 1, 1
	trig.Buttons[gamepad.Down], trig.Buttons[gamepad.RightTrigger] = 1, 0.05 // a trigger's noise at rest
	for _, c := range []struct {
		name    string
		last, s gamepad.State
		want    bool
	}{
		{"press", rest, down, true},
		{"stuck down", down, down, false},
		{"trigger noise while stuck", down, trig, false},
		{"another press while stuck", down, downA, true},
		{"release", down, rest, false},
	} {
		if got := newPress(c.last, c.s); got != c.want {
			t.Errorf("%s: newPress = %v, want %v", c.name, got, c.want)
		}
	}
}
