package main

// The launcher's look: Liquid Glass (MyGo's glass plugin) over a wallpaper of coloured light, with ColorOS 16's colours and metrics as
// suqi8/coui transcribes them (dark scheme: primary #247CFF; text at 90%, secondary text at 54%, summaries at 40% white; settings as
// grouped rows in rounded cards; switches with a primary track and a white knob).
// The whole view is zoomed (Context.SetZoom, 1.5 by default, 显示 → 界面缩放): it lays out in a window 1/zoom the panel's size and draws
// at the panel's resolution, so the layout takes its widths from c.Size() rather than from the 1920x1080 panel.

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

var (
	cBase      = ui.Hex("#04060c")
	cPrimary   = ui.Hex("#247CFF")
	cWhite     = ui.RGB(255, 255, 255)
	cText      = ui.RGBA(255, 255, 255, 0.90)
	cSecondary = ui.RGBA(255, 255, 255, 0.54)
	cSummary   = ui.RGBA(255, 255, 255, 0.40)
	cDivider   = ui.RGBA(255, 255, 255, 0.08)
	cSelRow    = ui.RGBA(255, 255, 255, 0.12)
	cSwitchOff = ui.RGBA(255, 255, 255, 0.20)
	cOn        = ui.Hex("#34c759")
	cDanger    = ui.Hex("#ff6b6b")
	cWarn      = ui.Hex("#ffb340")
	weekdays   = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
)

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// pane gives an element the launcher's material: glass, tinted toward a colour by its alpha, clearer as clarity grows
func pane(e ui.Element, radius float32, tint ui.Color, clarity float32) ui.Element {
	return e.Radius(radius).Material(glass.Glass{Tint: tint, Clarity: clarity})
}

// accent marks the chosen thing: glass tinted with the primary colour, growing a little while pressed
func accent(e ui.Element, col ui.Color) ui.Element {
	return e.Material(glass.Glass{Tint: col.Alpha(0.85), Interactive: true})
}

func (l *launcher) view(c *ui.Context) {
	c.SetZoom(l.zoom)
	if l.note != "" && time.Since(l.noteAt) > 4*time.Second {
		l.note = ""
	} else if l.note != "" {
		c.After(time.Second)
	}
	// dark whatever the system says: ROCKNIX's desktop has no colour scheme, so the theme was light and the glass lightened what it frosts
	t := *ui.DarkTheme().WithAccent(cPrimary)
	c.SetTheme(&t)
	l.slideBrightness, l.slideVolume, l.slideLED = float64(l.s.brightness), float64(l.s.volume), float64(l.s.ledBright)
	w, h := c.Size()
	ui.Box(c).Fill().Background(cBase).Clip().Children(func() {
		l.wallpaper(c, w, h)
		ui.Column(c).Fill().PaddingX(28).Padding(18).Gap(16).Children(func() {
			l.topBar(c)
			if l.focus == fGames {
				l.gamesView(c, w)
			} else {
				l.settingsView(c, w)
			}
			l.footer(c)
		})
		if l.running != "" {
			l.overlay(c, func() {
				ui.Text(c, "正在运行").FontSize(18).TextColor(cSecondary)
				ui.Text(c, l.running).FontSize(32).Bold().TextColor(cText)
			})
		} else if l.modal != nil {
			l.modal.view(l, c)
		}
	})
}

// wallpaper: three soft lights for the glass to bend and frost. On the games screen the largest takes the chosen game's colour, gliding
// to it as the choice moves
func (l *launcher) wallpaper(c *ui.Context, w, h float32) {
	a, b, d := ui.Hex("#2d5bff"), ui.Hex("#8b3dff"), ui.Hex("#00b3a4")
	if l.focus == fGames && len(l.games) > 0 {
		a = l.games[l.card].from
	}
	light := func(col ui.Color, x, y, size float32, key string) {
		blob := ui.Box(c).Absolute().Left(x).Top(y).Size(size, size)
		col = blob.AnimateColor(key, col, ui.Animation{Duration: 450 * time.Millisecond})
		blob.RadialGradient(ui.RadialGradient{Radius: size / 2, Stops: []ui.GradientStop{
			{Color: col.Alpha(0.75), At: 0}, {Color: col.Alpha(0.25), At: 0.45}, {Color: col.Alpha(0), At: 1}}})
	}
	light(a, -0.15*w, -0.3*h, 0.6*w, "a")
	light(b, 0.55*w, 0.25*h, 0.55*w, "b")
	light(d, 0.2*w, 0.55*h, 0.45*w, "d")
}

// ---- top bar: the clock, the two screens on one glass, the status on another ----

func (l *launcher) topBar(c *ui.Context) {
	s := &l.s
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Row(c).AlignItems(ui.Baseline).Gap(10).Grow(1).Basis(0).Children(func() {
			ui.Text(c, s.now.Format("15:04")).FontSize(34).Bold().TextColor(cText)
			ui.Text(c, fmt.Sprintf("%d月%d日 %s", s.now.Month(), s.now.Day(), weekdays[s.now.Weekday()])).FontSize(16).TextColor(cSecondary)
		})
		g := glass.NewGroup(c)
		g.Radius = 22
		g.Padding(4).Gap(3).Children(func() {
			for i, name := range []string{"游戏", "设置"} {
				active := (i == 0) == (l.focus == fGames)
				seg := ui.Box(c).Width(108).PaddingY(9).Radius(18).AlignItems(ui.Center)
				if active {
					accent(seg, cPrimary)
				}
				seg.Children(func() {
					ui.Text(c, name).FontSize(18).Bold().TextColor(pick(active, cWhite, cSecondary))
				})
				if g.Pane(seg).Clicked() {
					l.focus = pick(i == 0, fGames, fCategories)
				}
			}
		})
		ui.Row(c).Grow(1).Basis(0).Justify(ui.End).Children(func() {
			pane(ui.Row(c).AlignItems(ui.Center).Gap(13).PaddingX(16).PaddingY(9), 20, ui.Transparent, 0).Children(func() {
				if s.bt {
					ui.Text(c, "◈").FontSize(16).TextColor(cText)
				}
				wifiIcon(c, s.wifi.enabled && s.wifi.ssid != "", s.signal)
				if s.muted {
					ui.Text(c, "静音").FontSize(14).TextColor(cSecondary)
				}
				batteryIcon(c, s.bat)
				ui.Text(c, fmt.Sprintf("%d%%", s.bat.percent)).FontSize(16).TextColor(cText)
			})
		})
	})
}

// wifiIcon draws four bars, as many lit as the signal is strong
func wifiIcon(c *ui.Context, connected bool, signal int) {
	lit := 0
	if connected {
		lit = clamp((signal+24)/25, 1, 4)
	}
	ui.Row(c).AlignItems(ui.End).Gap(2).Height(16).Children(func() {
		for i := 0; i < 4; i++ {
			ui.Box(c).Width(4).Height(float32(4+i*4)).Radius(1.5).Background(pick(i < lit, cText, cSwitchOff))
		}
	})
}

// batteryIcon draws the cell filled to its charge: green charging, amber under 20%, red under 10%
func batteryIcon(c *ui.Context, b battery) {
	fill := cText
	switch {
	case b.charging:
		fill = cOn
	case b.percent < 10:
		fill = cDanger
	case b.percent < 20:
		fill = cWarn
	}
	ui.Row(c).AlignItems(ui.Center).Gap(1.5).Children(func() {
		ui.Box(c).Width(30).Height(16).Radius(5).Border(1.5, cSecondary).Padding(2).Children(func() {
			ui.Box(c).Width(float32(23*clamp(b.percent, 0, 100)/100)).Height(9).Radius(2).Background(fill)
		})
		ui.Box(c).Width(2).Height(6).Radius(1).Background(cSecondary)
	})
}

// ---- games: one poster card each (the game's Steam cover, which carries its own logo); the chosen one grows, lit at its rim in the
// game's colour, and its names show under the row, as a console's home screen does. A game without a cover is a glass card tinted with
// its colour, its name on it ----

func (l *launcher) gamesView(c *ui.Context, w float32) {
	const gap = 24
	games := l.games
	if len(games) == 0 {
		ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Text(c, "没有游戏").FontSize(24).Bold().TextColor(cText)
			ui.Text(c, "把游戏目录放进 "+strings.Join(gameRoots(), "、")).FontSize(15).TextColor(cSecondary)
		})
		return
	}
	cw := min(240, (w-56-gap*float32(len(games)-1))/float32(len(games)))
	ch := cw * 1.5 // Steam's library capsules are 600x900
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(26).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Gap(gap).Children(func() {
			for i, g := range games {
				sel := i == l.card
				cover := g.bmp
				// the chosen card grows by its size, not a transform: a clip or a rounded image under a transform is masked on the CPU
				// (MyGo's raster), which took 1.4 cores while the choice moved; axis-aligned rounded rectangles stay on the GPU
				card := ui.Box(c).Radius(22)
				grow := card.Animate("grow", pick[float32](sel, 1, 0), 160*time.Millisecond)
				k := 1 + 0.08*grow
				card.Size(cw*k, ch*k).Opacity(0.6 + 0.4*grow)
				if sel {
					card.Border(2, ui.RGBA(255, 255, 255, 0.7)).Shadow(0, 16, 48, 0, g.from.Alpha(0.7))
				}
				if cover == nil {
					card.Material(glass.Glass{Tint: g.from.Alpha(0.5)})
				}
				card.Children(func() {
					if cover != nil {
						ui.Image(c, cover).Size(cw*k, ch*k).Fit(ui.Cover).Radius(22)
						return
					}
					ui.Column(c).Fill().Padding(16).Justify(ui.End).Children(func() {
						ui.Text(c, g.title).FontSize(20).Bold().TextColor(cWhite)
					})
				})
				if card.Clicked() {
					if sel {
						l.launch(g)
					}
					l.card = i
				}
			}
		})
		g := games[l.card]
		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, g.title).FontSize(28).Bold().TextColor(cWhite)
				pane(ui.Box(c).PaddingX(9).PaddingY(3), 9, g.from.Alpha(0.5), 0).Children(func() {
					ui.Text(c, g.tag).FontSize(11).Bold().TextColor(cText)
				})
			})
			ui.Text(c, g.sub).FontSize(15).TextColor(cSecondary)
		})
	})
}

// ---- settings: the categories on a glass sidebar, the chosen one's rows in glass cards, a card for each section ----

func (l *launcher) settingsView(c *ui.Context, w float32) {
	cat := categories[l.cat]
	rows := cat.rows(l)
	if l.focus == fRows {
		rows = l.anchor(rows)
	}
	sliderW := max(160, min(380, w-28*2-210-18-360))
	ui.Row(c).Grow(1).Gap(18).AlignItems(ui.Stretch).Children(func() {
		pane(ui.Column(c).Width(210).Padding(9).Gap(3), 24, ui.Transparent, 0.3).Children(func() {
			for i, k := range categories {
				sel := i == l.cat
				item := ui.Row(c).AlignItems(ui.Center).Gap(10).PaddingX(13).PaddingY(11).Radius(16)
				switch {
				case sel && l.focus == fCategories:
					accent(item, cPrimary)
				case sel:
					item.Background(cSelRow)
				}
				item.Children(func() {
					ui.Text(c, k.icon).FontSize(16).Width(20).TextColor(pick(sel, cWhite, cSummary))
					ui.Text(c, k.name).FontSize(18).TextColor(pick(sel, cWhite, cSecondary))
				})
				if item.Clicked() {
					l.cat, l.sel, l.selKey, l.focus = i, 0, "", fCategories
				}
			}
		})
		ui.Column(c).Grow(1).Gap(8).Children(func() {
			ui.Text(c, cat.name).FontSize(25).Bold().TextColor(cText).PaddingX(6)
			ui.Scroll(c).Grow(1).Gap(6).Children(func() {
				// sections: the rows between two titles share a card, the title above it
				for start := 0; start < len(rows); {
					title := ""
					if rows[start].kind == kHeader {
						title = rows[start].label
						start++
					}
					end := start
					for end < len(rows) && rows[end].kind != kHeader {
						end++
					}
					if title != "" {
						ui.Text(c, title).FontSize(14).TextColor(cSecondary).PaddingX(10).Margin(3)
					}
					if end > start {
						pane(ui.Column(c).Padding(4), 20, ui.Transparent, 0.2).Children(func() {
							for i := start; i < end; i++ {
								if i > start {
									ui.Box(c).Height(1).MarginX(14).Background(cDivider)
								}
								l.rowView(c, rows[i], i, sliderW)
							}
						})
					}
					start = end
				}
			})
		})
	})
}

func (l *launcher) rowView(c *ui.Context, r row, i int, sliderW float32) {
	sel := l.focus == fRows && i == l.sel
	el := ui.Row(c).AlignItems(ui.Center).Gap(12).PaddingX(15).PaddingY(12).Radius(16).Background(pick(sel, cSelRow, ui.Transparent)).Children(func() {
		if sel {
			ui.Box(c).Width(3).Height(22).Radius(1.5).Background(cPrimary)
		}
		ui.Text(c, r.label).FontSize(18).TextColor(pick(r.danger, cDanger, cText)).Grow(1)
		switch r.kind {
		case kInfo, kAction:
			if r.value != "" {
				ui.Text(c, r.value).FontSize(15).TextColor(cSecondary)
			}
			if r.kind == kAction {
				ui.Text(c, "›").FontSize(22).TextColor(cSummary)
			}
		case kToggle:
			switchView(c, r.on)
		case kChoice:
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				if ui.Text(c, "‹").FontSize(22).TextColor(pick(sel, cPrimary, cSummary)).PaddingX(6).Clicked() {
					l.sel, l.selKey = i, r.id()
					r.adjust(-1)
				}
				ui.Text(c, r.value).FontSize(16).TextColor(cText).Width(160).TextAlign(ui.Center)
				if ui.Text(c, "›").FontSize(22).TextColor(pick(sel, cPrimary, cSummary)).PaddingX(6).Clicked() {
					l.sel, l.selKey = i, r.id()
					r.adjust(1)
				}
			})
		case kSlider:
			if ui.Slider(c, r.slider, 0, 100).Width(sliderW).Changed() {
				l.sel, l.selKey = i, r.id()
				r.set(int(*r.slider + 0.5))
			}
			ui.Text(c, r.value).FontSize(16).TextColor(cText).Width(52).TextAlign(ui.End)
		}
		if sel && r.alt != nil {
			pane(ui.Box(c).PaddingX(9).PaddingY(3), 9, ui.Transparent, 0).Children(func() {
				ui.Text(c, "X "+r.altLabel).FontSize(13).TextColor(cText)
			})
		}
	})
	if sel {
		el.ScrollIntoView()
	}
	if el.Clicked() {
		l.sel, l.selKey, l.focus = i, r.id(), fRows
		if r.act != nil {
			r.act()
		}
	}
}

// switchView draws ColorOS's switch: the knob slides to the end of a track that turns primary
func switchView(c *ui.Context, on bool) {
	track := ui.Row(c).Width(44).Height(25).Radius(12.5).Padding(2.5)
	pos := track.Animate("knob", pick[float32](on, 1, 0), 180*time.Millisecond)
	track.Background(cSwitchOff.Mix(cPrimary, pos)).Children(func() {
		ui.Box(c).Width(19 * pos)
		ui.Box(c).Width(20).Height(20).Radius(10).Background(cWhite).Shadow(0, 1.5, 4, 0, ui.RGBA(0, 0, 0, 0.25))
	})
}

// ---- footer: what the buttons do here, or the last action, on a glass pill ----

func (l *launcher) footer(c *ui.Context) {
	hint := ""
	switch {
	case l.busy != "":
		hint = l.busy + "…"
	case l.note != "":
		hint = l.note
	case l.focus == fGames:
		hint = "←→ 选择    A 开始    L/R 设置"
	case l.focus == fCategories:
		hint = "↑↓ 分类    → / A 进入    B 游戏    L/R 游戏"
	default:
		hint = "↑↓ 选择    ←→ 调节    A 确认    B 返回"
		if rows := categories[l.cat].rows(l); l.sel < len(rows) && rows[l.sel].alt != nil {
			hint += "    X " + rows[l.sel].altLabel
		}
	}
	ui.Row(c).Children(func() {
		pane(ui.Row(c).AlignItems(ui.Center).Gap(9).PaddingX(15).PaddingY(7), 17, ui.Transparent, 0).Children(func() {
			ui.Box(c).Width(6).Height(6).Radius(3).Background(pick(l.busy != "", cWarn, pick(l.note != "", cPrimary, cSummary)))
			ui.Text(c, hint).FontSize(14).TextColor(cText)
		})
	})
}

// overlay dims everything and shows a glass panel in the middle
func (l *launcher) overlay(c *ui.Context, content func()) {
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(2, 4, 10, 0.55)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		pane(ui.Column(c).Padding(28).Gap(16).AlignItems(ui.Center), 28, ui.RGBA(20, 24, 36, 0.35), 0).Children(content)
	})
}

// ---- confirmation ----

type confirmBox struct {
	question string
	yes      func()
	choice   int // 0 cancel, 1 confirm
}

func (m *confirmBox) press(l *launcher, b button) {
	switch b {
	case bLeft, bRight:
		m.choice ^= 1
	case bA:
		l.modal = nil
		if m.choice == 1 {
			m.yes()
		}
	case bB:
		l.modal = nil
	}
}

func (m *confirmBox) view(l *launcher, c *ui.Context) {
	l.overlay(c, func() {
		ui.Text(c, m.question).FontSize(25).Bold().TextColor(cText)
		ui.Row(c).Gap(12).Children(func() {
			for i, name := range []string{"取消", "确认"} {
				sel := i == m.choice
				b := ui.Box(c).Width(160).PaddingY(13).Radius(18).AlignItems(ui.Center)
				if sel {
					accent(b, pick(i == 1, cDanger, cPrimary))
				} else {
					b.Background(cSelRow)
				}
				b.Children(func() {
					ui.Text(c, name).FontSize(18).Bold().TextColor(pick(sel, cWhite, cText))
				})
				if b.Clicked() {
					m.choice = i
					m.press(l, bA)
				}
			}
		})
	})
}

// ---- on-screen keyboard: d-pad and A type, X shifts, B erases, + accepts, − cancels; a tap types too ----

var keyRows = [][]string{
	{"1", "2", "3", "4", "5", "6", "7", "8", "9", "0"},
	{"q", "w", "e", "r", "t", "y", "u", "i", "o", "p"},
	{"a", "s", "d", "f", "g", "h", "j", "k", "l", "@"},
	{"z", "x", "c", "v", "b", "n", "m", "-", "_", "."},
	{"⇧", "空格", "⌫", "完成"},
}

type keyboard struct {
	title    string
	text     string
	row, col int
	shift    bool
	done     func(string)
}

func newKeyboard(title string, done func(string)) *keyboard { return &keyboard{title: title, row: 1, done: done} }

func (k *keyboard) key(r, col int) string {
	s := keyRows[r][col]
	if k.shift && len(s) == 1 && s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0] - 32)
	}
	if k.shift && r == 0 {
		return string("!@#$%^&*()"[col])
	}
	return s
}

func (k *keyboard) typeKey(l *launcher, r, col int) {
	switch s := keyRows[r][col]; s {
	case "⇧":
		k.shift = !k.shift
	case "空格":
		k.text += " "
	case "⌫":
		if len(k.text) > 0 {
			k.text = k.text[:len(k.text)-1]
		}
	case "完成":
		l.modal = nil
		k.done(k.text)
	default:
		k.text += k.key(r, col)
	}
}

func (k *keyboard) press(l *launcher, b button) {
	switch b {
	case bUp:
		k.row = (k.row + len(keyRows) - 1) % len(keyRows)
	case bDown:
		k.row = (k.row + 1) % len(keyRows)
	case bLeft:
		k.col--
	case bRight:
		k.col++
	case bA:
		k.typeKey(l, k.row, k.col)
	case bX:
		k.shift = !k.shift
	case bB:
		if len(k.text) > 0 {
			k.text = k.text[:len(k.text)-1]
		}
	case bStart:
		l.modal = nil
		k.done(k.text)
	case bBack:
		l.modal = nil
	}
	n := len(keyRows[k.row])
	k.col = (k.col%n + n) % n
}

func (k *keyboard) view(l *launcher, c *ui.Context) {
	w, _ := c.Size()
	kw := min(64, (w-120-9*7)/10) // ten keys and their gaps inside the panel
	l.overlay(c, func() {
		ui.Text(c, k.title).FontSize(21).Bold().TextColor(cText)
		masked := ""
		for range k.text {
			masked += "•"
		}
		if len(k.text) > 0 {
			masked = masked[:len(masked)-len("•")] + k.text[len(k.text)-1:] // the last character shows
		}
		ui.Box(c).Width(kw*10+63).Padding(12).Radius(14).Background(ui.RGBA(0, 0, 0, 0.30)).Border(1.5, cPrimary).Children(func() {
			ui.Text(c, masked+"▏").FontSize(21).TextColor(cText)
		})
		ui.Column(c).Gap(7).Children(func() {
			for r, keys := range keyRows {
				ui.Row(c).Gap(7).Justify(ui.Center).Children(func() {
					for col := range keys {
						sel := r == k.row && col == k.col
						kwid := kw
						if r == len(keyRows)-1 {
							kwid = (kw*10 + 63 - 7*3) / 4
						}
						key := ui.Box(c).Width(kwid).Height(46).Radius(13).AlignItems(ui.Center).Justify(ui.Center)
						if sel {
							accent(key, cPrimary)
						} else {
							key.Background(cSelRow)
						}
						key.Children(func() {
							ui.Text(c, k.key(r, col)).FontSize(20).TextColor(pick(sel, cWhite, cText))
						})
						if key.Clicked() {
							k.row, k.col = r, col
							k.typeKey(l, r, col)
						}
					}
				})
			}
		})
		ui.Text(c, "A 输入    B 删除    X 大小写    + 完成    − 取消").FontSize(14).TextColor(cSecondary)
	})
}
