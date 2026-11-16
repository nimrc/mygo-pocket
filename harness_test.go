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
	l       *launcher
	tt      *ui.Tester
	updates chan func()
}

// newHarness starts the launcher on the games under roots, at the Flip 2's 1920x1080 and the default zoom
func newHarness(t *testing.T, answers map[string]string, roots ...string) *harness {
	t.Helper()
	sys.reset(answers)
	t.Setenv("MYGO_POCKET_GAMES", strings.Join(roots, string(os.PathListSeparator)))
	h := &harness{t: t, updates: make(chan func(), 256)}
	h.l = &launcher{zoom: 1.5, games: scanGames(), update: func(f func()) { h.updates <- f }}
	h.l.s.now = time.Date(2026, 11, 16, 9, 30, 0, 0, time.UTC)
	h.tt = ui.NewTester(h.l.view, 1920, 1080)
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

func (h *harness) shows(texts ...string) {
	h.t.Helper()
	for _, s := range texts {
		if !h.tt.HasText(s) {
			h.t.Errorf("%q not shown; texts %q", s, h.tt.Texts())
		}
	}
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
