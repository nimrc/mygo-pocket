// Probe: MyGo native UI as the Flip 2 launcher's shell under sway. A fullscreen window shows the battery and a brightness slider driven by the
// gamepad (d-pad up/down chooses a row, left/right changes it, A presses, B or Home quits). Brightness is set through ROCKNIX's `brightness`.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/egoist/mygo/ui"
)

type launcher struct {
	win        *mygo.Window
	battery    int
	charging   bool
	brightness float64
	row        int // 0 brightness, 1 quit
	pad        string
}

func readInt(path string) int {
	b, _ := os.ReadFile(path)
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func (l *launcher) poll() {
	b, _ := os.ReadFile("/sys/class/power_supply/battery/status")
	l.battery = readInt("/sys/class/power_supply/battery/capacity")
	l.charging = strings.TrimSpace(string(b)) == "Charging"
}

func (l *launcher) setBrightness(v float64) {
	l.brightness = max(5, min(100, v))
	exec.Command("brightness", "set", strconv.Itoa(int(l.brightness))).Run()
}

func (l *launcher) view(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(32).Children(func() {
		ui.Text(c, "Retroid Pocket Flip 2").FontSize(40).Bold()
		state := ""
		if l.charging {
			state = " (charging)"
		}
		ui.Text(c, fmt.Sprintf("Battery %d%%%s", l.battery, state)).FontSize(28)
		ui.Column(c).Width(600).Gap(8).Children(func() {
			label := "Brightness"
			if l.row == 0 {
				label = "> Brightness"
			}
			ui.Text(c, fmt.Sprintf("%s  %d%%", label, int(l.brightness))).FontSize(24)
			if ui.Slider(c, &l.brightness, 5, 100).Changed() {
				l.setBrightness(l.brightness)
			}
		})
		quit := "Quit"
		if l.row == 1 {
			quit = "> Quit"
		}
		ui.Text(c, quit).FontSize(24)
		ui.Text(c, "Pad: "+l.pad+"   D-pad: choose / change   A: press   B: quit").FontSize(16)
	})
}

// gamepad loop: polled at 60 Hz, applied on the main thread
func (l *launcher) input() {
	var last gamepad.State
	for range time.Tick(16 * time.Millisecond) {
		pads := gamepad.Pads()
		if len(pads) == 0 {
			continue
		}
		s := pads[0].State()
		name := pads[0].Name()
		up, down := s.JustPressed(last, gamepad.Up), s.JustPressed(last, gamepad.Down)
		left, right := s.JustPressed(last, gamepad.Left), s.JustPressed(last, gamepad.Right)
		a, b := s.JustPressed(last, gamepad.East), s.JustPressed(last, gamepad.South) || s.JustPressed(last, gamepad.Guide)
		last = s
		if !(up || down || left || right || a || b) && name == l.pad {
			continue
		}
		l.win.Update(func() {
			l.pad = name
			switch {
			case up || down:
				l.row ^= 1
			case (left || right) && l.row == 0:
				d := 5.0
				if left {
					d = -5
				}
				l.setBrightness(l.brightness + d)
			case b, a && l.row == 1:
				os.Exit(0)
			}
		})
	}
}

func main() {
	l := &launcher{brightness: float64(readInt("/sys/class/backlight/ae94000.dsi.0/brightness")) * 100 / 255}
	l.poll()
	mygo.App.WhenReady(func() {
		l.win = mygo.NewWindow(mygo.WindowOptions{Title: "nx2l", Frameless: true, Maximized: true, Content: ui.View(l.view)})
		go l.input()
		go func() {
			for range time.Tick(5 * time.Second) {
				l.win.Update(l.poll)
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
