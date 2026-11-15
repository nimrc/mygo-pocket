// nx2l launcher: the Retroid Pocket Flip 2's home screen on ROCKNIX. A fullscreen MyGo window in a sway session of its own (session.sh,
// sway-launcher.conf): the games (nx2linux ports and RetroArch) and the handheld's settings through ROCKNIX's tools (system.go).
//
// Gamepad: L/R switch between Games and Settings. Games: left/right choose, A starts. Settings: the category list on the left (up/down,
// right or A enters it), its rows on the right (up/down choose, left/right change, A does, X the row's second action, B back). Touch: a tap
// does what A does, a finger drags a slider (MYGO_TOUCH_POINTER). A game runs as a child process, fullscreen above the launcher; the
// launcher reads no input while it runs.
//
// NX2L_CTL=<path>: a FIFO of button names (up down left right a b x l r start back), one a line, pressed as the gamepad presses them,
// "find <category> <label>" and "type <text>" (the open keyboard), for testing on the device with screenshots.
package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/egoist/mygo/ui"
)

type game struct {
	id, title, sub, tag string
	from, to             ui.Color
	cmd                  string
}

var games = []game{
	{"silksong", "空洞骑士：丝之歌", "Hollow Knight: Silksong", "SWITCH · NX2LINUX", ui.Hex("#9f1239"), ui.Hex("#1c0710"), "DATA=/storage/nx2linux/silksong /storage/nx2linux/launch.sh"},
	{"hollowknight", "空洞骑士", "Hollow Knight", "SWITCH · NX2LINUX", ui.Hex("#1d4ed8"), ui.Hex("#0a1022"), "DATA=/storage/nx2linux/hollowknight /storage/nx2linux/launch.sh"},
	{"retroarch", "RetroArch", "模拟器", "RETRO", ui.Hex("#6d28d9"), ui.Hex("#120a24"), "retroarch"},
}

type button int

const (
	bUp button = iota
	bDown
	bLeft
	bRight
	bA
	bB
	bX
	bL
	bR
	bStart
	bBack
)

var buttonNames = map[string]button{"up": bUp, "down": bDown, "left": bLeft, "right": bRight, "a": bA, "b": bB, "x": bX, "l": bL, "r": bR, "start": bStart, "back": bBack}

type focus int

const (
	fGames focus = iota
	fCategories
	fRows
)

type launcher struct {
	win     *mygo.Window
	s       state
	focus   focus
	card    int
	cat     int
	sel     int
	running string // the game playing, "" when none
	playing atomic.Bool
	busy    string // an action in progress
	note    string // the last action's outcome, shown for a few seconds
	noteAt  time.Time
	modal   modal // a keyboard or a confirmation over everything, nil when none

	slideBrightness, slideVolume float64 // the sliders' values, following the state unless a finger moves them
}

// modal takes the buttons while it is open
type modal interface {
	press(l *launcher, b button)
	view(l *launcher, c *ui.Context)
}

// do runs an action off the main thread, then reads the state again and shows its outcome
func (l *launcher) do(what string, f func() string) {
	if l.busy != "" {
		l.say("请稍候：" + l.busy)
		return
	}
	l.busy, l.note = what, ""
	go func() {
		out := f()
		log.Printf("%s: %s", what, firstLine(out))
		l.win.Update(func() { l.busy = ""; l.say(firstLine(out)) }) // the outcome at once; reading the whole state again takes seconds
		w, nets := readWifi(), cachedWifi()                          // the network first: an action on it changed what the list shows
		l.win.Update(func() {
			if !l.s.wifiScanning {
				l.s.wifi, l.s.nets = w, markInUse(nets, w.ssid)
			}
		})
		l.refresh(true)
	}()
}

func (l *launcher) say(note string) { l.note, l.noteAt = note, time.Now() }

func (l *launcher) launch(g game) {
	if l.running != "" {
		return
	}
	l.running, l.note = g.title, ""
	l.playing.Store(true)
	go func() {
		cmd := exec.Command("bash", "-c", g.cmd)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		err := cmd.Run()
		log.Printf("%s ended: %v", g.id, err)
		time.Sleep(500 * time.Millisecond) // the game's last presses are not the launcher's
		l.playing.Store(false)
		l.refresh(true)
		l.win.Update(func() { l.running = "" })
	}()
}

// press is every button press, from the gamepad or NX2L_CTL, on the main thread
func (l *launcher) press(b button) {
	if l.running != "" {
		return
	}
	if l.modal != nil {
		l.modal.press(l, b)
		return
	}
	if b == bL || b == bR {
		if l.focus == fGames {
			l.focus = fCategories
		} else {
			l.focus = fGames
		}
		return
	}
	switch l.focus {
	case fGames:
		switch b {
		case bLeft:
			l.card = (l.card + len(games) - 1) % len(games)
		case bRight:
			l.card = (l.card + 1) % len(games)
		case bA, bStart:
			l.launch(games[l.card])
		case bDown:
			l.focus = fCategories
		}
	case fCategories:
		switch b {
		case bUp:
			l.cat, l.sel = (l.cat+len(categories)-1)%len(categories), 0
		case bDown:
			l.cat, l.sel = (l.cat+1)%len(categories), 0
		case bRight, bA:
			l.focus, l.sel = fRows, 0
		case bB:
			l.focus = fGames
		}
	case fRows:
		rows := categories[l.cat].rows(l)
		if len(rows) == 0 {
			l.focus = fCategories
			return
		}
		l.sel = clamp(l.sel, 0, len(rows)-1)
		r := rows[l.sel]
		switch b {
		case bUp:
			l.sel = (l.sel + len(rows) - 1) % len(rows)
		case bDown:
			l.sel = (l.sel + 1) % len(rows)
		case bLeft, bRight:
			d := 1
			if b == bLeft {
				d = -1
			}
			if r.adjust != nil {
				r.adjust(d)
			} else if b == bLeft {
				l.focus = fCategories
			}
		case bA:
			if r.act != nil {
				r.act()
			} else if r.adjust != nil {
				r.adjust(1)
			}
		case bX:
			if r.alt != nil {
				r.alt()
			}
		case bB:
			l.focus = fCategories
		}
	}
}

// ---- input ----

// input polls every pad at 60 Hz (the first is not always the one in the player's hands); a d-pad direction held repeats after 400 ms,
// every 110 ms. Nothing is read while a game runs.
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
			held[b] = now.Add(110 * time.Millisecond)
			return true
		}
		return false
	}
	for range time.Tick(16 * time.Millisecond) {
		pads := gamepad.Pads()
		var s gamepad.State
		for _, p := range pads {
			ps := p.State()
			for i := range s.Buttons {
				s.Buttons[i] = max(s.Buttons[i], ps.Buttons[i])
			}
		}
		if l.playing.Load() {
			last, held = s, map[gamepad.Button]time.Time{}
			continue
		}
		var bs []button
		for b, gb := range map[button]gamepad.Button{bUp: gamepad.Up, bDown: gamepad.Down, bLeft: gamepad.Left, bRight: gamepad.Right} {
			if repeat(s, gb) {
				bs = append(bs, b)
			}
		}
		for b, gb := range map[button]gamepad.Button{bA: gamepad.East, bB: gamepad.South, bX: gamepad.North, bL: gamepad.LeftShoulder, bR: gamepad.RightShoulder, bStart: gamepad.Start, bBack: gamepad.Back} {
			if s.JustPressed(last, gb) {
				bs = append(bs, b)
			}
		}
		last = s
		if len(bs) > 0 {
			l.win.Update(func() {
				for _, b := range bs {
					l.press(b)
				}
			})
		}
	}
}

// find selects the first row of a category whose label or value holds text (NX2L_CTL "find <category> <text>"); when none does, the
// focus stays on the settings' categories, where the presses after it cannot start a game or change a setting
func (l *launcher) find(cat, text string) {
	l.modal = nil
	for i, k := range categories {
		if k.name != cat {
			continue
		}
		for j, r := range k.rows(l) {
			if r.kind != kHeader && (strings.Contains(r.label, text) || strings.Contains(r.value, text)) {
				l.cat, l.focus, l.sel = i, fRows, j
				return
			}
		}
	}
	log.Printf("NX2L_CTL find %s %s: no such row", cat, text)
	l.focus = fCategories
}

// control reads button names from NX2L_CTL, a FIFO it makes; "find <category> <label>" selects a row
func (l *launcher) control(path string) {
	os.Remove(path)
	if err := syscall.Mkfifo(path, 0600); err != nil {
		log.Printf("NX2L_CTL %s: %v", path, err)
		return
	}
	for {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if b, ok := buttonNames[line]; ok {
				l.win.Update(func() { l.press(b) })
			} else if cat, label, ok := strings.Cut(strings.TrimPrefix(line, "find "), " "); ok && strings.HasPrefix(line, "find ") {
				l.win.Update(func() { l.find(cat, label) })
			} else if text, ok := strings.CutPrefix(line, "type "); ok {
				l.win.Update(func() {
					if k, ok := l.modal.(*keyboard); ok {
						k.text += text
					}
				})
			}
		}
		f.Close()
	}
}

func main() {
	l := &launcher{}
	mygo.App.WhenReady(func() {
		l.win = mygo.NewWindow(mygo.WindowOptions{Title: "nx2l", Frameless: true, Maximized: true, TouchPointer: true, Content: ui.View(l.view)})
		go l.refresh(true)
		go l.input()
		if p := os.Getenv("NX2L_CTL"); p != "" {
			go l.control(p)
		}
		go func() {
			for i := 1; ; i++ {
				time.Sleep(2 * time.Second)
				if !l.playing.Load() {
					l.refresh(i%15 == 0) // the slow readings every 30 s
				}
			}
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
