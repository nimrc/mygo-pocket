package main

// The games are found, not listed: each directory under the roots ($MYGO_POCKET_GAMES, colon-separated; default /storage/games:/storage/nx2linux) is a card
// when it holds a pocket.json (schema/pocket.schema.json) or an nx2linux data pack (porter's nx2a-data.json), its id the directory's name.
// A card is one of the kinds below, so a game is installed on any handheld by copying its directory over, and removed by deleting it.
//
//	nx2linux   a Switch port: launch.sh at the root runs the directory's dmhost on it (found by nx2a-data.json alone)
//	mygo       a MyGo app in the directory, exec[0] relative to it (its window goes fullscreen above the launcher in the same sway)
//	native     any other program in the directory, exec[0] relative to it: an SDL game, a script
//	command    a program on PATH, exec[0] as is: retroarch, a system tool
//	retroarch  one retro game: RetroArch runs the core on the ROM in the directory

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

const manifestName = "pocket.json"

// manifest is pocket.json (schema/pocket.schema.json)
type manifest struct {
	Schema   string            `json:"$schema"`
	Kind     string            `json:"kind"`
	Title    string            `json:"title"`
	Subtitle string            `json:"subtitle"`
	Tag      string            `json:"tag"`
	Color    string            `json:"color"`
	Cover    string            `json:"cover"`
	Exec     []string          `json:"exec"`
	Env      map[string]string `json:"env"`
	Core     string            `json:"core"`
	ROM      string            `json:"rom"`
	Order    int               `json:"order"`
	SteamApp int               `json:"steam_appid"` // fetch-art.sh's cover
	Hidden   bool              `json:"hidden"`
}

// dataPack is the part of porter's nx2a-data.json the launcher reads
type dataPack struct {
	Name    string `json:"name"`
	TitleID string `json:"title_id"`
	Display string `json:"display"`
}

type game struct {
	id, dir, title, sub, tag string
	from, to                 ui.Color
	argv                     []string
	env                      []string
	cover                    string
	bmp                      *ui.Bitmap
	order                    int
}

func gameRoots() []string {
	if s := os.Getenv("MYGO_POCKET_GAMES"); s != "" {
		return filepath.SplitList(s)
	}
	return []string{"/storage/games", "/storage/nx2linux"}
}

// scanGames reads the roots, ordered by the manifests' order, then title
func scanGames() []game {
	var gs []game
	for _, root := range gameRoots() {
		ents, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			if g, ok := readGame(root, filepath.Join(root, e.Name())); ok {
				gs = append(gs, g)
			}
		}
	}
	slices.SortStableFunc(gs, func(a, b game) int {
		if a.order != b.order {
			return a.order - b.order
		}
		return strings.Compare(strings.ToLower(a.title), strings.ToLower(b.title))
	})
	return gs
}

func readGame(root, dir string) (game, bool) {
	var m manifest
	var p dataPack
	hasM := readJSON(filepath.Join(dir, manifestName), &m)
	hasP := readJSON(filepath.Join(dir, "nx2a-data.json"), &p)
	if !hasM && !hasP || m.Hidden {
		return game{}, false
	}
	if m.Kind == "" && hasP {
		m.Kind = "nx2linux"
	}
	g := game{id: filepath.Base(dir), dir: dir, title: m.Title, sub: m.Subtitle, tag: m.Tag, order: m.Order}
	fail := func(why string) (game, bool) { log.Printf("%s: %s", dir, why); return game{}, false }
	switch m.Kind {
	case "nx2linux":
		if !hasP {
			return fail("an nx2linux card needs nx2a-data.json")
		}
		g.title = cmp(g.title, p.Name)
		if g.sub == "" && g.title != p.Name {
			g.sub = p.Name
		}
		g.tag = cmp(g.tag, "SWITCH · NX2LINUX")
		g.argv = []string{filepath.Join(root, "launch.sh")}
		g.env = append(g.env, "DATA="+dir)
	case "mygo", "native":
		if len(m.Exec) == 0 {
			return fail("a " + m.Kind + " card needs exec")
		}
		g.tag = cmp(g.tag, map[string]string{"mygo": "MYGO", "native": "APP"}[m.Kind])
		g.argv = append([]string{filepath.Join(dir, m.Exec[0])}, m.Exec[1:]...)
	case "command":
		if len(m.Exec) == 0 {
			return fail("a command card needs exec")
		}
		g.tag = cmp(g.tag, "SYSTEM")
		g.argv = m.Exec
	case "retroarch":
		core := findCore(m.Core)
		if core == "" || m.ROM == "" {
			return fail(fmt.Sprintf("core %q not found or no rom", m.Core))
		}
		g.tag = cmp(g.tag, "RETRO · "+strings.ToUpper(m.Core))
		g.argv = []string{"retroarch", "-L", core, filepath.Join(dir, m.ROM)}
	default:
		return fail(fmt.Sprintf("unknown kind %q", m.Kind))
	}
	if g.title == "" {
		return fail("no title")
	}
	g.env = append(g.env, "GAME_DIR="+dir)
	for k, v := range m.Env {
		g.env = append(g.env, k+"="+v)
	}
	g.from = defaultColor(g.id)
	if m.Color != "" {
		g.from = ui.Hex(m.Color)
	}
	g.to = g.from.Mix(ui.Hex("#000000"), 0.85)
	for _, c := range []string{m.Cover, "cover.jpg", "cover.png"} {
		if c != "" && exists(filepath.Join(dir, c)) {
			g.cover = filepath.Join(dir, c)
			break
		}
	}
	g.bmp = loadCover(g.cover)
	return g, true
}

// findCore finds a libretro core by name (snes9x, mgba) where ROCKNIX keeps them; $RETROARCH_CORES first
func findCore(name string) string {
	if name == "" {
		return ""
	}
	dirs := append(filepath.SplitList(os.Getenv("RETROARCH_CORES")), "/tmp/cores", "/storage/.config/retroarch/cores", "/usr/lib/libretro")
	for _, d := range dirs {
		if p := filepath.Join(d, name+"_libretro.so"); d != "" && exists(p) {
			return p
		}
	}
	return ""
}

func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(data, v); err != nil {
		log.Printf("%s: %v", path, err)
		return false
	}
	return true
}

func cmp(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// a game without a colour gets one of these, the same one every time
var palette = []string{"#9f1239", "#1d4ed8", "#b45309", "#6d28d9", "#047857", "#0e7490", "#be185d", "#4d7c0f"}

func defaultColor(id string) ui.Color {
	h := fnv.New32a()
	h.Write([]byte(id))
	return ui.Hex(palette[h.Sum32()%uint32(len(palette))])
}

// covers holds the decoded posters by path, decoded again when the file changes
var (
	covers   = map[string]coverCache{}
	coversMu sync.Mutex
)

type coverCache struct {
	mod time.Time
	bmp *ui.Bitmap
}

func loadCover(path string) *ui.Bitmap {
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	coversMu.Lock()
	defer coversMu.Unlock()
	if c, ok := covers[path]; ok && c.mod.Equal(st.ModTime()) {
		return c.bmp
	}
	var bmp *ui.Bitmap
	if data, err := os.ReadFile(path); err == nil {
		if bmp, err = ui.DecodeBitmap(data); err != nil {
			log.Printf("cover %s: %v", path, err)
		}
	}
	covers[path] = coverCache{st.ModTime(), bmp}
	return bmp
}

// setGames swaps in a fresh scan, the chosen card staying on its game
func (l *launcher) setGames(gs []game) {
	id := ""
	if l.card < len(l.games) {
		id = l.games[l.card].id
	}
	l.games, l.card = gs, 0
	for i, g := range gs {
		if g.id == id {
			l.card = i
		}
	}
}
