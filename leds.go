package main

// The analog sticks' RGB rings (four multicolour LEDs a stick, rgb:l1-4 and rgb:r1-4). ROCKNIX drives them in a mode (led.color: off,
// battery, or rgb with analogsticks.led's "<brightness> <right r g b> <left r g b>") through ledcontrol. Its battery mode writes brightness
// 255 at each colour change, so the launcher keeps the rings at its own brightness (mygo-pocket.led.brightness) every few seconds, game or
// not; a ring at 0 is left dark (off, or the low-battery blink).

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var ledModes = []struct{ id, name string }{{"off", "关闭"}, {"battery", "电量"}, {"rgb", "颜色"}}

var ledColors = []struct {
	name    string
	r, g, b int
}{{"白", 255, 255, 255}, {"红", 255, 0, 0}, {"橙", 255, 80, 0}, {"黄", 255, 200, 0}, {"绿", 0, 255, 0}, {"青", 0, 255, 255},
	{"蓝", 0, 0, 255}, {"紫", 160, 0, 255}, {"粉", 255, 40, 120}}

// ledLevel is the rings' brightness in percent, for the goroutine that holds it
var ledLevel atomic.Int32

func stickLEDs() []string {
	ls, _ := filepath.Glob("/sys/class/leds/rgb:[lr][1-4]")
	return ls
}

func readLEDMode() string {
	if len(stickLEDs()) == 0 {
		return ""
	}
	return getSetting("led.color")
}

func readLEDBrightness() int {
	if n, err := strconv.Atoi(getSetting("mygo-pocket.led.brightness")); err == nil {
		return clamp(n, 1, 100)
	}
	return 100
}

// readLEDColor is analogsticks.led's right-stick colour as an index of ledColors, -1 when it is none of them
func readLEDColor() int {
	f := strings.Fields(getSetting("analogsticks.led"))
	if len(f) == 7 {
		for i, c := range ledColors {
			if f[1] == strconv.Itoa(c.r) && f[2] == strconv.Itoa(c.g) && f[3] == strconv.Itoa(c.b) {
				return i
			}
		}
	}
	return -1
}

func level(percent int) int { return (clamp(percent, 1, 100)*255 + 50) / 100 }

func setLEDMode(mode string) { run(10*time.Second, "ledcontrol", mode) }

func setLEDColor(i, percent int) {
	c := ledColors[i]
	setSetting("analogsticks.led", fmt.Sprintf("%d %d %d %d %d %d %d", level(percent), c.r, c.g, c.b, c.r, c.g, c.b))
	run(5*time.Second, "ledcontrol", "rgb")
}

// holdLEDBrightness puts every lit ring at percent
func holdLEDBrightness(percent int) {
	want := strconv.Itoa(level(percent))
	for _, l := range stickLEDs() {
		if b := readFile(l + "/brightness"); b != "0" && b != want {
			os.WriteFile(l+"/brightness", []byte(want), 0)
		}
	}
}
