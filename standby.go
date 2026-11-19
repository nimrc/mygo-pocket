package main

// Auto standby: after a while on the launcher with no input, the handheld goes to standby the way its power key does. EmulationStation
// had this (its screensaver); the launcher that replaced it did not. Only the launcher's idle time counts: a game, or another window in
// front, is never put to standby.

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

const standbySetting = "mygo-pocket.standby" // minutes, 0 off

// standbyChoices are the minutes 设置 → 电源 → 自动待机 steps through
var standbyChoices = []int{0, 5, 10, 30}

var (
	standbyUnit   = time.Minute                               // a test shortens it
	standbyWarn   = 30 * time.Second                          // the warning before standby, any button cancels it
	fakeSuspended = "/var/run/power-fake-suspend-active.flag" // rocknix-fake-suspend's while it is on
)

// clock is monotonic: it stops while the handheld is suspended, so the time asleep is not idle time on waking
var clockStart = time.Now()

func clock() time.Duration { return time.Since(clockStart) }

func readStandby() int {
	v := getSetting(standbySetting)
	if v == "" {
		return 10
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return n // any number of minutes set by hand is kept, not only the choices
	}
	return 10
}

func standbyName(m int) string {
	if m == 0 {
		return "关闭"
	}
	return fmt.Sprintf("%d 分钟", m)
}

// standby does what the power key does: ROCKNIX's fake suspend (screen and input off, cores parked) where real suspend is off, as on the
// Snapdragon handhelds, and a real suspend elsewhere. Both are started detached: they return when the handheld wakes.
var standby = func() {
	if getSetting("system.suspendmode") == "off" {
		shell(3*time.Second, "/usr/bin/rocknix-fake-suspend power >/dev/null 2>&1 &")
		return
	}
	shell(3*time.Second, "systemctl suspend >/dev/null 2>&1 &")
}

// touch marks input: the idle time starts again
func (l *launcher) touch() { l.active.Store(int64(clock())) }

// checkStandby runs off the main thread every couple of seconds: it warns before the idle time runs out and then goes to standby
func (l *launcher) checkStandby() {
	m := int(l.standbyMin.Load())
	// in a fake suspend the launcher still runs: its power toggle would wake the handheld again
	if m <= 0 || l.playing.Load() || l.behind.Load() || exists(fakeSuspended) {
		l.touch()
		return
	}
	idle := clock() - time.Duration(l.active.Load())
	limit := time.Duration(m) * standbyUnit
	switch {
	case idle >= limit:
		l.touch() // after waking the idle time starts again
		l.update(func() {
			if _, ok := l.modal.(*standbyBox); ok {
				l.modal = nil
			}
		})
		standby()
	case idle >= limit-standbyWarn:
		at := time.Now().Add(limit - idle)
		l.update(func() {
			if l.modal == nil {
				l.modal = &standbyBox{at: at}
			}
		})
	}
}

// standbyBox is the warning: any button cancels the standby
type standbyBox struct{ at time.Time }

func (m *standbyBox) press(l *launcher, b button) { l.modal = nil } // press has already marked the input

func (m *standbyBox) view(l *launcher, c *ui.Context) {
	left := max(0, int(time.Until(m.at).Round(time.Second)/time.Second))
	l.overlay(c, func() {
		ui.Text(c, "即将待机").FontSize(pick[float32](l.compact, 24, 27)).Bold().TextColor(cText)
		ui.Text(c, fmt.Sprintf("%d 秒后进入待机", left)).FontSize(pick[float32](l.compact, 18, 19)).TextColor(cText)
		ui.Text(c, "按任意键取消").FontSize(pick[float32](l.compact, 15, 14)).TextColor(cSecondary)
	})
}

func (l *launcher) setStandby(d int) {
	i := slices.Index(standbyChoices, int(l.standbyMin.Load()))
	if i < 0 { // a value set by hand: step from the nearest choice
		i = 2
	}
	m := standbyChoices[clamp(i+d, 0, len(standbyChoices)-1)]
	l.standbyMin.Store(int32(m))
	l.touch()
	go setSetting(standbySetting, strconv.Itoa(m))
}
