// mygo-pocket: a handheld's home screen on ROCKNIX, in place of EmulationStation. A fullscreen MyGo window in a sway session of its own
// (dist/session.sh, dist/sway.conf): the games found under $MYGO_POCKET_GAMES (games.go) and the handheld's settings through ROCKNIX's tools (system.go).
//
// Gamepad: L/R switch between Games and Settings. Games: left/right choose, A starts. Settings: the category list on the left (up/down,
// right or A enters it), its rows on the right (up/down choose, left/right change, A does, X the row's second action, B back). Touch: a tap
// does what A does, a finger drags a slider (MYGO_TOUCH_POINTER). A game runs as a child process, fullscreen above the launcher; the
// launcher reads no input while it runs.
//
// MYGO_POCKET_CTL=<path>: a FIFO of button names (up down left right a b x l r start back), one a line, pressed as the gamepad presses them,
// "find <category> <label>", "game <id>" (chooses that card) and "type <text>" (the open keyboard), for testing on the device with screenshots.
package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"runtime/pprof"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/game/gamepad"
	"github.com/egoist/mygo/ui"
)

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
	update  func(func()) // runs on the main thread: the window's Update, the tests' queue
	s       state
	games   []game
	focus   focus
	card    int
	editing bool // left/right change the chosen row's value (A on a choice or a slider) until A or B
	cat     int
	sel     int
	selKey  string // the selected row's label: rows of a list that changes (networks, devices) move, and the selection follows its row
	running string // the game playing, "" when none
	playing atomic.Bool
	behind  atomic.Bool // another window is in front (a game started outside the launcher): its presses are not the launcher's
	busy    string // an action in progress
	note    string // the last action's outcome, shown for a few seconds
	noteAt  time.Time
	modal   modal // a keyboard or a confirmation over everything, nil when none

	slideBrightness, slideVolume, slideLED float64 // the sliders' values, following the state unless a finger moves them
	zoom                         float32 // the view's zoom (显示 → 界面缩放), kept as mygo-pocket.ui.zoom in ROCKNIX's settings
	compact                      bool    // a small panel's layout (view.go), as the last frame found the panel

	wall    *ui.Bitmap // the wallpaper, drawn for wallKey
	wallKey wallKey
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
		l.update(func() { l.busy = ""; l.say(firstLine(out)) }) // the outcome at once; reading the whole state again takes seconds
		w, nets := readWifi(), cachedWifi() // the lists an action changes first: the whole state takes seconds
		bt := bluetoothOn()
		var devs []btDevice
		if bt {
			devs = readBtDevices()
		}
		l.update(func() {
			if !l.s.wifiScanning {
				l.s.wifi, l.s.nets = w, markInUse(nets, w.ssid)
			}
			l.s.bt, l.s.btDevs = bt, devs
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
		cmd := exec.Command(g.argv[0], g.argv[1:]...)
		cmd.Dir, cmd.Env = g.dir, append(os.Environ(), g.env...)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		err := cmd.Run()
		log.Printf("%s ended: %v", g.id, err)
		time.Sleep(500 * time.Millisecond) // the game's last presses are not the launcher's
		l.playing.Store(false)
		l.refresh(true)
		l.update(func() { l.running = "" })
	}()
}

// anchor keeps the selection on the row it was on when the rows changed under it, then remembers that row's label
func (l *launcher) anchor(rows []row) []row {
	if len(rows) == 0 {
		l.sel, l.selKey = 0, ""
		return rows
	}
	l.sel = clamp(l.sel, 0, len(rows)-1)
	if l.selKey != "" && rows[l.sel].id() != l.selKey {
		for i, r := range rows {
			if r.kind != kHeader && r.id() == l.selKey {
				l.sel = i
				break
			}
		}
	}
	l.selKey = rows[l.sel].id()
	return rows
}

// press is every button press, from the gamepad or MYGO_POCKET_CTL, on the main thread
func (l *launcher) press(b button) {
	if l.running != "" {
		return
	}
	if l.modal != nil {
		l.modal.press(l, b)
		return
	}
	if l.focus != fRows {
		l.editing = false
	}
	if b == bL || b == bR {
		l.editing = false
		if l.focus == fGames {
			l.focus = fCategories
		} else {
			l.focus = fGames
		}
		return
	}
	switch l.focus {
	case fGames:
		n := len(l.games)
		switch b {
		case bLeft:
			if n > 0 {
				l.card = (l.card + n - 1) % n
			}
		case bRight:
			if n > 0 {
				l.card = (l.card + 1) % n
			}
		case bA, bStart:
			if n > 0 {
				l.launch(l.games[l.card])
			}
		case bDown:
			l.focus = fCategories
		}
	case fCategories:
		if l.compact { // the categories are a row of icons
			switch b {
			case bLeft:
				b = bUp
			case bRight:
				b = bDown
			case bDown:
				b = bA
			case bUp:
				return
			}
		}
		switch b {
		case bUp:
			l.cat, l.sel, l.selKey = (l.cat+len(categories)-1)%len(categories), 0, ""
		case bDown:
			l.cat, l.sel, l.selKey = (l.cat+1)%len(categories), 0, ""
		case bRight, bA:
			l.focus, l.sel, l.selKey = fRows, 0, ""
		case bB:
			l.focus = fGames
		}
	case fRows:
		rows := l.anchor(categories[l.cat].rows(l))
		if len(rows) == 0 {
			l.focus = fCategories
			return
		}
		r := rows[l.sel]
		defer func() { l.selKey = ""; l.anchor(categories[l.cat].rows(l)) }() // the row the press moved to
		if l.editing && r.adjust != nil { // A chose this value to change: left/right change it, A or B is done
			switch b {
			case bLeft:
				r.adjust(-1)
			case bRight:
				r.adjust(1)
			case bA, bB:
				l.editing = false
			}
			return
		}
		l.editing = false
		switch b {
		case bUp, bDown: // to the next row there is, over the section titles
			d := 1
			if b == bUp {
				d = len(rows) - 1
			}
			for k := 0; k < len(rows); k++ {
				if l.sel = (l.sel + d) % len(rows); rows[l.sel].kind != kHeader {
					break
				}
			}
		case bLeft, bB: // left is always back: a value changes only after A
			l.focus = fCategories
		case bA:
			if r.act != nil {
				r.act()
			} else if r.adjust != nil {
				l.editing = true
			}
		case bX:
			if r.alt != nil {
				r.alt()
			}
		}
	}
}

// ---- input ----

// input polls every pad at 60 Hz (the first is not always the one in the player's hands); a d-pad direction held repeats after 400 ms,
// every 110 ms. Nothing is read while a game runs.
func (l *launcher) input() {
	var last gamepad.State
	held := map[gamepad.Button]time.Time{}
	wait := false
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
		if l.playing.Load() || l.behind.Load() {
			last, held, wait = s, map[gamepad.Button]time.Time{}, true
			continue
		}
		if wait { // back in front: nothing until every button the game held is up
			if anyPressed(s) {
				last = s
				continue
			}
			wait = false
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
			l.update(func() {
				for _, b := range bs {
					l.press(b)
				}
			})
		}
	}
}

// find selects the first row of a category whose label or value holds text (MYGO_POCKET_CTL "find <category> <text>"); when none does, the
// focus stays on the settings' categories, where the presses after it cannot start a game or change a setting
func (l *launcher) find(cat, text string) {
	l.modal = nil
	for i, k := range categories {
		if k.name != cat {
			continue
		}
		for j, r := range k.rows(l) {
			if r.kind != kHeader && (strings.Contains(r.label, text) || strings.Contains(r.value, text)) {
				l.cat, l.focus, l.sel, l.selKey = i, fRows, j, r.id()
				return
			}
		}
	}
	log.Printf("MYGO_POCKET_CTL find %s %s: no such row", cat, text)
	l.focus = fCategories
}

// control reads button names from MYGO_POCKET_CTL, a FIFO it makes; "find <category> <label>" selects a row
func (l *launcher) control(path string) {
	os.Remove(path)
	if err := syscall.Mkfifo(path, 0600); err != nil {
		log.Printf("MYGO_POCKET_CTL %s: %v", path, err)
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
				l.update(func() { l.press(b) })
			} else if cat, label, ok := strings.Cut(strings.TrimPrefix(line, "find "), " "); ok && strings.HasPrefix(line, "find ") {
				l.update(func() { l.find(cat, label) })
			} else if id, ok := strings.CutPrefix(line, "game "); ok {
				l.update(func() {
					for i, g := range l.games {
						if g.id == id {
							l.focus, l.card, l.editing = fGames, i, false
						}
					}
				})
			} else if text, ok := strings.CutPrefix(line, "type "); ok {
				l.update(func() {
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
	if path := os.Getenv("MYGO_POCKET_PPROF"); path != "" { // a CPU profile of the first 20 s, for finding what a frame costs
		if f, err := os.Create(path); err == nil && pprof.StartCPUProfile(f) == nil {
			time.AfterFunc(20*time.Second, func() { pprof.StopCPUProfile(); f.Close(); log.Printf("profile written to %s", path) })
		}
	}
	l := &launcher{zoom: readZoom(), games: scanGames()}
	mygo.App.WhenReady(func() {
		// Transparent on OpenGL ES (GDK_GL=gles, as Panfrost needs): GTK 3 draws an opaque GL area from a renderbuffer, which it can copy to
		// the window only with framebuffer blits, and it finds none on ES; it then read every frame back from the GPU (glReadPixels), 150 ms a
		// frame on the RG ARC. A transparent area's texture it draws with GL.
		gles := strings.Contains(os.Getenv("GDK_GL"), "gles")
		l.win = mygo.NewWindow(mygo.WindowOptions{Title: "mygo-pocket", Frameless: true, Maximized: true, TouchPointer: true, Transparent: gles, Content: ui.View(l.view)})
		l.update = l.win.Update
		l.win.OnBlur(func() { l.behind.Store(true); log.Print("behind another window: gamepad ignored") })
		l.win.OnFocus(func() { l.behind.Store(false); log.Print("in front: gamepad read") })
		go l.refresh(true)
		go l.input()
		go func() { // the stick lights at the chosen brightness, whatever ROCKNIX's battery mode writes, game or not
			ledLevel.Store(int32(readLEDBrightness()))
			for range time.Tick(2 * time.Second) {
				holdLEDBrightness(int(ledLevel.Load()))
			}
		}()
		if p := os.Getenv("MYGO_POCKET_CTL"); p != "" {
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

// anyPressed tells whether a button is held (a trigger past its press point counts; its noise at rest does not)
func anyPressed(s gamepad.State) bool {
	for i := range s.Buttons {
		if s.Pressed(gamepad.Button(i)) {
			return true
		}
	}
	return false
}
