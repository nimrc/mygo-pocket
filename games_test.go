package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// TestKinds reads a root holding a card of each kind and the directories that are not cards
func TestKinds(t *testing.T) {
	root, cores := t.TempDir(), t.TempDir()
	t.Setenv("RETROARCH_CORES", cores)
	os.WriteFile(filepath.Join(cores, "snes9x_libretro.so"), nil, 0o644)
	var cover bytes.Buffer
	png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 2, 3)))

	gameDir(t, root, "pack", map[string]any{"nx2a-data.json": map[string]any{"name": "Pack"}, "dmhost": "", "cover.png": cover.String()})
	gameDir(t, root, "named-pack", map[string]any{"nx2a-data.json": map[string]any{"name": "Original"}, "dmhost": "",
		"pocket.json": map[string]any{"title": "译名", "color": "#123456", "order": -1}})
	gameDir(t, root, "app", map[string]any{"pocket.json": map[string]any{"kind": "mygo", "title": "App", "exec": []string{"bin/app", "--x"}, "env": map[string]string{"K": "v"}}})
	gameDir(t, root, "tool", map[string]any{"pocket.json": map[string]any{"kind": "native", "title": "Tool", "exec": []string{"run.sh"}, "tag": "MINE"}})
	gameDir(t, root, "ra", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "RetroArch", "exec": []string{"retroarch"}}})
	gameDir(t, root, "snes", map[string]any{"pocket.json": map[string]any{"kind": "retroarch", "title": "SNES", "core": "snes9x", "rom": "g.sfc"}})

	// not cards
	gameDir(t, root, "copying", map[string]any{"nx2a-data.json": map[string]any{"name": "No dmhost"}})
	gameDir(t, root, "hidden", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "Hidden", "exec": []string{"x"}, "hidden": true}})
	gameDir(t, root, ".dot", map[string]any{"pocket.json": map[string]any{"kind": "command", "title": "Dot", "exec": []string{"x"}}})
	gameDir(t, root, "no-exec", map[string]any{"pocket.json": map[string]any{"kind": "native", "title": "No exec"}})
	gameDir(t, root, "no-core", map[string]any{"pocket.json": map[string]any{"kind": "retroarch", "title": "No core", "core": "mgba", "rom": "g.gba"}})
	gameDir(t, root, "unknown", map[string]any{"pocket.json": map[string]any{"kind": "flatpak", "title": "Unknown"}})
	gameDir(t, root, "untitled", map[string]any{"pocket.json": map[string]any{"kind": "command", "exec": []string{"x"}}})
	gameDir(t, root, "broken", map[string]any{"pocket.json": "{"})
	gameDir(t, root, "empty", nil)

	t.Setenv("MYGO_POCKET_GAMES", root+string(os.PathListSeparator)+filepath.Join(root, "missing"))
	gs := scanGames()
	var ids []string
	byID := map[string]game{}
	for _, g := range gs {
		ids = append(ids, g.id)
		byID[g.id] = g
	}
	if want := []string{"named-pack", "app", "pack", "ra", "snes", "tool"}; !slices.Equal(ids, want) {
		t.Fatalf("cards %q, want %q", ids, want)
	}
	for _, c := range []struct {
		id, title, sub, tag string
		argv                []string
		env                 []string
	}{
		{"pack", "Pack", "", "SWITCH · NX2LINUX", []string{filepath.Join(root, "launch.sh")}, []string{"DATA=" + filepath.Join(root, "pack")}},
		{"named-pack", "译名", "Original", "SWITCH · NX2LINUX", []string{filepath.Join(root, "launch.sh")}, nil},
		{"app", "App", "", "MYGO", []string{filepath.Join(root, "app", "bin/app"), "--x"}, []string{"K=v"}},
		{"tool", "Tool", "", "MINE", []string{filepath.Join(root, "tool", "run.sh")}, nil},
		{"ra", "RetroArch", "", "SYSTEM", []string{"retroarch"}, nil},
		{"snes", "SNES", "", "RETRO · SNES9X", []string{"retroarch", "-L", filepath.Join(cores, "snes9x_libretro.so"), filepath.Join(root, "snes", "g.sfc")}, nil},
	} {
		g := byID[c.id]
		if g.title != c.title || g.sub != c.sub || g.tag != c.tag || !slices.Equal(g.argv, c.argv) {
			t.Errorf("%s: %q %q %q %q, want %q %q %q %q", c.id, g.title, g.sub, g.tag, g.argv, c.title, c.sub, c.tag, c.argv)
		}
		for _, e := range append(c.env, "GAME_DIR="+g.dir) {
			if !slices.Contains(g.env, e) {
				t.Errorf("%s: env %q lacks %q", c.id, g.env, e)
			}
		}
	}
	if g := byID["pack"]; g.bmp == nil || !strings.HasSuffix(g.cover, "cover.png") {
		t.Errorf("pack's cover %q not decoded", g.cover)
	}
	if g := byID["named-pack"]; g.from != ui.Hex("#123456") {
		t.Errorf("named-pack's colour %v", g.from)
	}
	if a, b := byID["app"].from, scanGames()[1].from; a != b {
		t.Errorf("a card's own colour changed between scans: %v %v", a, b)
	}
}

// TestManifestsMatchSchema: the examples and the README's manifest pass schema/pocket.schema.json, and what the launcher would refuse
// fails it
func TestManifestsMatchSchema(t *testing.T) {
	c := jsonschema.NewCompiler()
	sch, err := c.Compile("schema/pocket.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	valid := func(name string, data []byte) error {
		v, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return sch.Validate(v)
	}
	examples, _ := filepath.Glob("examples/*/pocket.json")
	if len(examples) == 0 {
		t.Fatal("no examples")
	}
	for _, p := range examples {
		data, _ := os.ReadFile(p)
		if err := valid(p, data); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	readme, _ := os.ReadFile("README.md")
	_, rest, _ := strings.Cut(string(readme), "```json\n")
	block, _, ok := strings.Cut(rest, "```")
	if !ok {
		t.Fatal("no manifest in README.md")
	}
	if err := valid("README.md", []byte(block)); err != nil {
		t.Errorf("README.md's manifest: %v", err)
	}
	for _, bad := range []map[string]any{
		{"kind": "flatpak", "title": "x"},
		{"kind": "native", "title": "x"},
		{"kind": "retroarch", "title": "x", "core": "snes9x"},
		{"kind": "command", "title": "x", "exec": []string{}},
		{"title": "x", "color": "red"},
		{"title": "x", "titel": "typo"},
	} {
		data, _ := json.Marshal(bad)
		if valid("bad", data) == nil {
			t.Errorf("%s passes the schema", data)
		}
	}
}
