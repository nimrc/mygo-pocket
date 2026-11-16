package main

// The tests drive the launcher as the handheld does, without one: ui.NewTester draws it in memory at the panel's size, presses go through
// press as the gamepad's do, and run is a fake handheld that answers ROCKNIX's tools and records what the launcher asked of them.
// MYGO_POCKET_SHOTS=<dir> saves the screens the tests take (h.shot) as PNGs.

import (
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// fakeHandheld stands for ROCKNIX's tools. It is set once for the package: a background command of one test may still run in the next.
type fakeHandheld struct {
	mu      sync.Mutex
	cmds    []string
	answers map[string]string // a part of a command line → its output
}

var sys = &fakeHandheld{}

func TestMain(m *testing.M) {
	run = sys.run
	os.Exit(m.Run())
}

func (f *fakeHandheld) run(_ time.Duration, name string, args ...string) string {
	line := strings.Join(append([]string{name}, args...), " ")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, line)
	for k, v := range f.answers {
		if strings.Contains(line, k) {
			return v
		}
	}
	return ""
}

func (f *fakeHandheld) reset(answers map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds, f.answers = nil, answers
}

// count is how many command lines holding part ran
func (f *fakeHandheld) count(part string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.cmds {
		if strings.Contains(c, part) {
			n++
		}
	}
	return n
}

func (f *fakeHandheld) ran(part string) bool { return f.count(part) > 0 }

type harness struct {
	t       *testing.T
	p       panel
	l       *launcher
	tt      *ui.Tester
	updates chan func()
}

// panel is a handheld's screen the e2e tests run on
type panel struct {
	name  string
	w, h  int
	touch bool
}

// panels: the Retroid Pocket Flip 2 (16:9, touch) and the Anbernic RG ARC (4:3, buttons only)
var panels = []panel{{"flip2", 1920, 1080, true}, {"rgarc", 640, 480, false}}

// onPanels runs an e2e test on each panel; a test that taps skips the panels without a touchscreen
func onPanels(t *testing.T, taps bool, test func(t *testing.T, p panel)) {
	for _, p := range panels {
		t.Run(p.name, func(t *testing.T) {
			if taps && !p.touch {
				t.Skip("no touchscreen")
			}
			test(t, p)
		})
	}
}

// newHarness starts the launcher on the games under roots, on panel p at the default zoom
func newHarness(t *testing.T, p panel, answers map[string]string, roots ...string) *harness {
	t.Helper()
	sys.reset(answers)
	t.Setenv("MYGO_POCKET_GAMES", strings.Join(roots, string(os.PathListSeparator)))
	h := &harness{t: t, p: p, updates: make(chan func(), 256)}
	h.l = &launcher{zoom: 1.5, games: scanGames(), update: func(f func()) { h.updates <- f }}
	h.l.s.now = time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	h.tt = ui.NewTester(h.l.view, p.w, p.h)
	return h
}

// press presses buttons as the gamepad does, a frame after each
func (h *harness) press(names ...string) {
	h.t.Helper()
	for _, n := range names {
		b, ok := buttonNames[n]
		if !ok {
			h.t.Fatalf("no button %q", n)
		}
		h.l.press(b)
		h.tt.Frame()
	}
}

// toCategory goes to the settings and moves n categories on: down the sidebar, or right along a compact panel's icons
func (h *harness) toCategory(n int) {
	h.t.Helper()
	h.press("r")
	for range n {
		h.press(pick(h.l.compact, "right", "down"))
	}
}

// hasCard reports whether the launcher has a card of id
func (h *harness) hasCard(id string) bool {
	for _, g := range h.l.games {
		if g.id == id {
			return true
		}
	}
	return false
}

func (h *harness) click(text string) {
	h.t.Helper()
	if err := h.tt.Click(text); err != nil {
		h.t.Fatalf("click %q: %v; texts %q", text, err, h.tt.Texts())
	}
}

// until runs what background work hands the main thread until ok holds
func (h *harness) until(what string, ok func() bool) {
	h.t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		h.drain()
		h.tt.Frame()
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("%s: timed out; texts %q", what, h.tt.Texts())
		}
	}
}

func (h *harness) drain() {
	for {
		select {
		case f := <-h.updates:
			f()
		default:
			return
		}
	}
}

// shows checks that each text is in the frame and inside the panel, so a layout too wide or tall for a 4:3 panel fails
func (h *harness) shows(texts ...string) {
	h.t.Helper()
	for _, s := range texts {
		if !h.tt.HasText(s) {
			h.t.Errorf("%q not shown; texts %q", s, h.tt.Texts())
		} else if !h.onPanel(s) {
			h.t.Errorf("%q is off the %dx%d panel", s, h.p.w, h.p.h)
		}
	}
}

// onPanel reports whether a text containing s lies inside the panel
func (h *harness) onPanel(s string) bool {
	for _, text := range h.tt.Texts() {
		if !strings.Contains(text, s) {
			continue
		}
		if r, ok := h.tt.Find(text); ok && r.X >= 0 && r.Y >= 0 && r.X+r.W <= float32(h.p.w)+0.5 && r.Y+r.H <= float32(h.p.h)+0.5 {
			return true
		}
	}
	return false
}

func (h *harness) hides(texts ...string) {
	h.t.Helper()
	for _, s := range texts {
		if h.tt.HasText(s) {
			h.t.Errorf("%q shown", s)
		}
	}
}

func (h *harness) shot(name string) {
	dir := os.Getenv("MYGO_POCKET_SHOTS")
	if dir == "" {
		return
	}
	h.t.Helper()
	os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, strings.ReplaceAll(h.t.Name(), "/", "_")+"-"+name+".png"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.tt.Image()); err != nil {
		h.t.Fatal(err)
	}
}

// gameDir makes a card's directory: name → content, a value that is not a string written as JSON
func gameDir(t *testing.T, root, id string, files map[string]any) string {
	t.Helper()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, v := range files {
		data, ok := v.(string)
		if !ok {
			b, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			data = string(b)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
