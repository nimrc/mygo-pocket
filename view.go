package main

// The launcher's look: Liquid Glass (MyGo's glass plugin) over a wallpaper of coloured light, with ColorOS 16's colours and metrics as
// suqi8/coui transcribes them (dark scheme: primary #247CFF; text at 90%, secondary text at 54%, summaries at 40% white; settings as
// grouped rows in rounded cards; switches with a primary track and a white knob).
// The whole view is zoomed (Context.SetZoom, 显示 → 界面缩放): it lays out in a window 1/zoom the panel's size and draws at the panel's
// resolution, so the layout takes its widths from c.Size() rather than from the panel.
//
// A panel under 720 pixels on its short side (the RG ARC's 4-inch 640x480) is compact: it lays out in about 600x450 at the default zoom,
// so text draws at 1.5–1.7 times the size the 1080p layout would give it there; the games show three cards at a time, and the settings
// put their categories in a row of icons above the rows instead of a sidebar beside them. Its GPU (a Mali-G52) is a tenth of the
// Flip 2's, and a pane of glass blurs what is under it in passes of its own that took it about 25 ms each: nothing is glass there, the
// panels are translucent fills and what is chosen the primary colour, with a light rim.

import (
	"fmt"
	"image"
	"image/color"
	"math"
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
	cFlat      = ui.RGBA(22, 26, 40, 0.62) // a compact panel's fill, where a larger one is glass
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

// pane gives an element the launcher's material: glass, tinted toward a colour by its alpha, clearer as clarity grows; on a compact
// panel a translucent fill, as glass blurs what is under it in passes of its own, which a Mali-G52 cannot afford a panel each
func (l *launcher) pane(e ui.Element, radius float32, tint ui.Color, clarity float32) ui.Element {
	if l.compact {
		return e.Radius(radius).Background(cFlat.Mix(tint, float32(tint.A)/255)).Border(1, cDivider)
	}
	return e.Radius(radius).Material(glass.Glass{Tint: tint, Clarity: clarity})
}

// accent marks the chosen thing: glass tinted with the primary colour, growing a little while pressed; on a compact panel the colour
func (l *launcher) accent(e ui.Element, col ui.Color) ui.Element {
	if l.compact {
		return e.Background(col).Border(1, ui.RGBA(255, 255, 255, 0.22))
	}
	return e.Material(glass.Glass{Tint: col.Alpha(0.85), Interactive: true})
}

func (l *launcher) view(c *ui.Context) {
	// 界面缩放 is relative to a 1080p panel, 1.5x laying out in 1280x720 on the Flip 2; a compact panel's to 675 pixels, 1.5x laying out
	// in 600x450 on a 640x480 RG ARC
	pw, ph := c.Size()
	short := min(pw, ph) * c.Zoom()
	l.compact = short < 720
	c.SetZoom(l.zoom * short / pick[float32](l.compact, 675, 1080))
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
	ui.Box(c).Fill().Clip().Children(func() { // no background: the wallpaper is opaque, and every fill the window's size costs the G52 ~30 ms
		l.wallpaper(c, w, h)
		col := ui.Column(c).Fill()
		if l.compact {
			col.PaddingX(16).Padding(10).Gap(10)
		} else {
			col.PaddingX(28).Padding(18).Gap(16)
		}
		col.Children(func() {
			l.topBar(c)
			switch {
			case l.focus == fGames && l.compact:
				l.gamesCompact(c, w, h)
			case l.focus == fGames:
				l.gamesView(c, w)
			case l.compact:
				l.settingsCompact(c, w)
			default:
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
// to it as the choice moves. They are drawn into a small bitmap, which the GPU stretches over the window, and drawn again only as the
// colour or the window changes: three radial gradients the window's size took a Mali-G52 longer than everything else on it.
func (l *launcher) wallpaper(c *ui.Context, w, h float32) {
	a := ui.Hex("#2d5bff")
	if l.focus == fGames && len(l.games) > 0 {
		a = l.games[l.card].from
	}
	// under the flow (an absolute element paints over it, as in CSS: the lights tinted the posters and the glass had only black to frost),
	// and out of hit testing
	box := ui.Box(c).Absolute().Left(0).Top(0).Size(w, h).ZIndex(-1).IgnoreInput(true)
	a = box.AnimateColor("a", a, ui.Animation{Duration: 450 * time.Millisecond})
	if key := (wallKey{a, w, h}); l.wall == nil || l.wallKey != key {
		img := paintWallpaper(a, w, h)
		if l.wall == nil {
			l.wall = ui.NewBitmap(img)
		} else {
			l.wall.Update(img)
		}
		l.wallKey = key
	}
	box.Children(func() {
		ui.Image(c, l.wall).Size(w, h).Fit(ui.Cover)
	})
}

// wallKey is what the wallpaper's bitmap was drawn for
type wallKey struct {
	a    ui.Color
	w, h float32
}

// paintWallpaper draws the lights over the base colour, 192 pixels wide: they are soft enough that the GPU's stretching shows no steps
func paintWallpaper(a ui.Color, w, h float32) *image.RGBA {
	bw := 192
	bh := max(1, int(math.Round(float64(float32(bw)*h/w))))
	img := image.NewRGBA(image.Rect(0, 0, bw, bh))
	type light struct {
		col        ui.Color
		x, y, size float32
	}
	lights := []light{
		{a, -0.15 * w, -0.3 * h, 0.6 * w},
		{ui.Hex("#8b3dff"), 0.55 * w, 0.25 * h, 0.55 * w},
		{ui.Hex("#00b3a4"), 0.2 * w, 0.55 * h, 0.45 * w},
	}
	for j := 0; j < bh; j++ {
		for i := 0; i < bw; i++ {
			x, y := (float32(i)+0.5)*w/float32(bw), (float32(j)+0.5)*h/float32(bh)
			r, g, b := float32(cBase.R), float32(cBase.G), float32(cBase.B)
			for _, lt := range lights {
				rad := lt.size / 2
				d := float32(math.Hypot(float64(x-lt.x-rad), float64(y-lt.y-rad))) / rad
				// the gradient's stops: 75% at the middle, 25% at 0.45, none at the rim
				var al float32
				switch {
				case d < 0.45:
					al = 0.75 - 0.5*d/0.45
				case d < 1:
					al = 0.25 * (1 - (d-0.45)/0.55)
				}
				al *= float32(lt.col.A) / 255
				r += (float32(lt.col.R) - r) * al
				g += (float32(lt.col.G) - g) * al
				b += (float32(lt.col.B) - b) * al
			}
			img.SetRGBA(i, j, color.RGBA{uint8(r + 0.5), uint8(g + 0.5), uint8(b + 0.5), 255})
		}
	}
	return img
}

// ---- top bar: the clock, the two screens on one glass, the status on another ----

func (l *launcher) topBar(c *ui.Context) {
	s := &l.s
	k := l.compact
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Row(c).AlignItems(ui.Baseline).Gap(pick[float32](k, 8, 10)).Grow(1).Basis(0).Children(func() {
			ui.Text(c, s.now.Format("15:04")).FontSize(pick[float32](k, 26, 34)).Bold().TextColor(cText)
			ui.Text(c, fmt.Sprintf("%d月%d日 %s", s.now.Month(), s.now.Day(), weekdays[s.now.Weekday()])).FontSize(pick[float32](k, 14, 16)).TextColor(cSecondary)
		})
		var bar ui.Element
		paneOf := func(e ui.Element) ui.Element { return e }
		if k {
			bar = ui.Row(c).Radius(20).Background(cFlat).Border(1, cDivider)
		} else {
			g := glass.NewGroup(c)
			g.Radius = 22
			bar, paneOf = g.Element, g.Pane
		}
		bar.Padding(pick[float32](k, 3, 4)).Gap(3).Children(func() {
			for i, name := range []string{"游戏", "设置"} {
				active := (i == 0) == (l.focus == fGames)
				seg := ui.Box(c).Width(pick[float32](k, 80, 108)).PaddingY(pick[float32](k, 7, 9)).Radius(pick[float32](k, 17, 18)).AlignItems(ui.Center)
				if active {
					l.accent(seg, cPrimary)
				}
				seg.Children(func() {
					ui.Text(c, name).FontSize(pick[float32](k, 17, 18)).Bold().TextColor(pick(active, cWhite, cSecondary))
				})
				if paneOf(seg).Clicked() {
					l.focus = pick(i == 0, fGames, fCategories)
				}
			}
		})
		ui.Row(c).Grow(1).Basis(0).Justify(ui.End).Children(func() {
			l.pane(ui.Row(c).AlignItems(ui.Center).Gap(pick[float32](k, 9, 13)).PaddingX(pick[float32](k, 12, 16)).PaddingY(pick[float32](k, 7, 9)), 20, ui.Transparent, 0).Children(func() {
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
		l.noGames(c)
		return
	}
	cw := min(240, (w-56-gap*float32(len(games)-1))/float32(len(games)))
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(26).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Gap(gap).Children(func() {
			for i := range games {
				l.poster(c, i, cw)
			}
		})
		l.gameNames(c, games[l.card], 28, 15)
	})
}

// gamesCompact shows the cards three at a time, as large as the panel's height allows, the window following the choice; a ‹ or › at a
// side tells there are more that way
func (l *launcher) gamesCompact(c *ui.Context, w, h float32) {
	const gap = 16
	games := l.games
	if len(games) == 0 {
		l.noGames(c)
		return
	}
	cw := min(h*0.5/1.5, (w-32-2*28-gap*2)/3) // 2:3 posters half the height; three across with the arrows beside them
	n := min(3, len(games))
	first := clamp(l.card-n/2, 0, len(games)-n)
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Justify(ui.Center).Gap(gap).Children(func() {
			ui.Text(c, "‹").FontSize(28).Width(28).TextAlign(ui.Center).TextColor(pick(first > 0, cSecondary, ui.Transparent))
			for i := first; i < first+n; i++ {
				l.poster(c, i, cw)
			}
			ui.Text(c, "›").FontSize(28).Width(28).TextAlign(ui.Center).TextColor(pick(first+n < len(games), cSecondary, ui.Transparent))
		})
		l.gameNames(c, games[l.card], 24, 16)
	})
}

func (l *launcher) noGames(c *ui.Context) {
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "没有游戏").FontSize(24).Bold().TextColor(cText)
		ui.Text(c, "把游戏目录放进 "+strings.Join(gameRoots(), "、")).FontSize(15).TextColor(cSecondary)
	})
}

// poster is game i's card, cw wide
func (l *launcher) poster(c *ui.Context, i int, cw float32) {
	g := l.games[i]
	ch := cw * 1.5 // Steam's library capsules are 600x900
	sel := i == l.card
	cover := g.bmp
	// the chosen card grows by its size, not a transform: a clip or a rounded image under a transform is masked on the CPU (MyGo's
	// raster), which took 1.4 cores while the choice moved; axis-aligned rounded rectangles stay on the GPU
	radius := pick[float32](l.compact, 16, 22)
	card := ui.Box(c).Radius(radius).Label(g.title) // a poster has no text: the label names it to taps in tests and to screen readers
	grow := card.Animate("grow", pick[float32](sel, 1, 0), 160*time.Millisecond)
	k := 1 + 0.08*grow
	card.Size(cw*k, ch*k).Opacity(0.6 + 0.4*grow)
	if sel {
		card.Border(2, ui.RGBA(255, 255, 255, 0.7)).Shadow(0, 16, 48, 0, g.from.Alpha(0.7))
	}
	if cover == nil {
		if l.compact {
			card.Background(g.from.Alpha(0.55))
		} else {
			card.Material(glass.Glass{Tint: g.from.Alpha(0.5)})
		}
	}
	card.Children(func() {
		if cover != nil {
			ui.Image(c, cover).Size(cw*k, ch*k).Fit(ui.Cover).Radius(radius)
			return
		}
		ui.Column(c).Fill().Padding(pick[float32](l.compact, 12, 16)).Justify(ui.End).Children(func() {
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

// gameNames: the chosen game's title and tag, and its subtitle under them
func (l *launcher) gameNames(c *ui.Context, g game, title, sub float32) {
	ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, g.title).FontSize(title).Bold().TextColor(cWhite)
			l.pane(ui.Box(c).PaddingX(9).PaddingY(3), 9, g.from.Alpha(0.5), 0).Children(func() {
				ui.Text(c, g.tag).FontSize(pick[float32](l.compact, 13, 11)).Bold().TextColor(cText)
			})
		})
		ui.Text(c, g.sub).FontSize(sub).TextColor(cSecondary)
	})
}

// ---- settings: the categories on a glass sidebar, the chosen one's rows in glass cards, a card for each section ----

func (l *launcher) settingsView(c *ui.Context, w float32) {
	cat := categories[l.cat]
	rows := categories[l.cat].rows(l)
	if l.focus == fRows {
		rows = l.anchor(rows)
	}
	sliderW := max(160, min(380, w-28*2-210-18-360))
	ui.Row(c).Grow(1).Gap(18).AlignItems(ui.Stretch).Children(func() {
		l.pane(ui.Column(c).Width(210).Padding(9).Gap(3), 24, ui.Transparent, 0.3).Children(func() {
			for i, k := range categories {
				sel := i == l.cat
				item := ui.Row(c).AlignItems(ui.Center).Gap(10).PaddingX(13).PaddingY(11).Radius(16)
				switch {
				case sel && l.focus == fCategories:
					l.accent(item, cPrimary)
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
			l.sections(c, rows, sliderW)
		})
	})
}

// settingsCompact puts the categories in a row of icons, the chosen one's name beside them, and its rows under them across the panel
func (l *launcher) settingsCompact(c *ui.Context, w float32) {
	cat := categories[l.cat]
	rows := cat.rows(l)
	if l.focus == fRows {
		rows = l.anchor(rows)
	}
	sliderW := max(120, min(240, w-32-24-150-60))
	ui.Column(c).Grow(1).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
			l.pane(ui.Row(c).Padding(4).Gap(2), 18, ui.Transparent, 0).Children(func() {
				for i, k := range categories {
					sel := i == l.cat
					item := ui.Box(c).Width(46).Height(38).Radius(14).AlignItems(ui.Center).Justify(ui.Center).Label(k.name)
					switch {
					case sel && l.focus == fCategories:
						l.accent(item, cPrimary)
					case sel:
						item.Background(cSelRow)
					}
					item.Children(func() {
						ui.Text(c, k.icon).FontSize(20).TextColor(pick(sel, cWhite, cSecondary))
					})
					if item.Clicked() {
						l.cat, l.sel, l.selKey, l.focus = i, 0, "", fCategories
					}
				}
			})
			ui.Text(c, cat.name).FontSize(22).Bold().TextColor(cText)
		})
		l.sections(c, rows, sliderW)
	})
}

// sections: the rows between two titles share a card, the title above it
func (l *launcher) sections(c *ui.Context, rows []row, sliderW float32) {
	k := l.compact
	ui.Scroll(c).Grow(1).Gap(6).Children(func() {
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
				ui.Text(c, title).FontSize(pick[float32](k, 15, 14)).TextColor(cSecondary).PaddingX(10).Margin(3)
			}
			if end > start {
				l.pane(ui.Column(c).Padding(4), pick[float32](k, 16, 20), ui.Transparent, 0.2).Children(func() {
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
}

func (l *launcher) rowView(c *ui.Context, r row, i int, sliderW float32) {
	k := l.compact
	sel := l.focus == fRows && i == l.sel
	editing := sel && l.editing
	el := ui.Row(c).AlignItems(ui.Center).Gap(pick[float32](k, 10, 12)).PaddingX(pick[float32](k, 12, 15)).PaddingY(pick[float32](k, 10, 12)).Radius(pick[float32](k, 13, 16)).
		Background(pick(editing, cPrimary.Alpha(0.22), pick(sel, cSelRow, ui.Transparent)))
	if editing {
		el.Border(1.5, cPrimary.Alpha(0.8))
	}
	el.Children(func() {
		if sel {
			ui.Box(c).Width(3).Height(22).Radius(1.5).Background(cPrimary)
		}
		ui.Text(c, r.label).FontSize(pick[float32](k, 19, 18)).TextColor(pick(r.danger, cDanger, cText)).Grow(1)
		switch r.kind {
		case kInfo, kAction:
			if r.value != "" {
				ui.Text(c, r.value).FontSize(pick[float32](k, 16, 15)).TextColor(cSecondary)
			}
			if r.kind == kAction {
				ui.Text(c, "›").FontSize(22).TextColor(cSummary)
			}
		case kToggle:
			switchView(c, r.on)
		case kChoice:
			ui.Row(c).AlignItems(ui.Center).Gap(pick[float32](k, 4, 6)).Children(func() {
				if ui.Text(c, "‹").FontSize(22).TextColor(pick(editing, cPrimary, cSummary)).PaddingX(6).Clicked() {
					l.sel, l.selKey = i, r.id()
					r.adjust(-1)
				}
				ui.Text(c, r.value).FontSize(pick[float32](k, 17, 16)).TextColor(cText).Width(pick[float32](k, 120, 160)).TextAlign(ui.Center)
				if ui.Text(c, "›").FontSize(22).TextColor(pick(editing, cPrimary, cSummary)).PaddingX(6).Clicked() {
					l.sel, l.selKey = i, r.id()
					r.adjust(1)
				}
			})
		case kSlider:
			if ui.Slider(c, r.slider, 0, 100).Width(sliderW).Changed() {
				l.sel, l.selKey = i, r.id()
				r.set(int(*r.slider + 0.5))
			}
			ui.Text(c, r.value).FontSize(pick[float32](k, 17, 16)).TextColor(cText).Width(52).TextAlign(ui.End)
		}
		if sel && r.alt != nil {
			l.pane(ui.Box(c).PaddingX(9).PaddingY(3), 9, ui.Transparent, 0).Children(func() {
				ui.Text(c, "X "+r.altLabel).FontSize(pick[float32](k, 14, 13)).TextColor(cText)
			})
		}
	})
	if sel {
		el.ScrollIntoView()
	}
	if el.Clicked() {
		l.sel, l.selKey, l.focus, l.editing = i, r.id(), fRows, false
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
	case l.focus == fCategories && l.compact:
		hint = "←→ 分类    ↓ / A 进入    B 游戏"
	case l.focus == fCategories:
		hint = "↑↓ 分类    → / A 进入    B 游戏    L/R 游戏"
	case l.editing:
		hint = "←→ 调节    A / B 完成"
	default:
		hint = "↑↓ 选择    A 确认    ← / B 返回"
		if rows := categories[l.cat].rows(l); l.sel < len(rows) && rows[l.sel].adjust != nil && rows[l.sel].act == nil {
			hint = "↑↓ 选择    A 修改    ← / B 返回"
		}
		if rows := categories[l.cat].rows(l); l.sel < len(rows) && rows[l.sel].alt != nil {
			hint += "    X " + rows[l.sel].altLabel
		}
	}
	ui.Row(c).Children(func() {
		l.pane(ui.Row(c).AlignItems(ui.Center).Gap(9).PaddingX(15).PaddingY(pick[float32](l.compact, 6, 7)), 17, ui.Transparent, 0).Children(func() {
			ui.Box(c).Width(6).Height(6).Radius(3).Background(pick(l.busy != "", cWarn, pick(l.note != "", cPrimary, cSummary)))
			ui.Text(c, hint).FontSize(pick[float32](l.compact, 15, 14)).TextColor(cText)
		})
	})
}

// overlay dims everything and shows a glass panel in the middle
func (l *launcher) overlay(c *ui.Context, content func()) {
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(2, 4, 10, 0.55)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		p := ui.Column(c).Padding(pick[float32](l.compact, 18, 28)).Gap(pick[float32](l.compact, 12, 16)).AlignItems(ui.Center)
		if l.compact {
			p.Radius(24).Background(ui.RGBA(20, 24, 36, 0.92)).Border(1, cDivider) // over the dimmed screen, where glass would show little
		} else {
			p.Radius(28).Material(glass.Glass{Tint: ui.RGBA(20, 24, 36, 0.35)})
		}
		p.Children(content)
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
				b := ui.Box(c).Width(pick[float32](l.compact, 140, 160)).PaddingY(13).Radius(18).AlignItems(ui.Center)
				if sel {
					l.accent(b, pick(i == 1, cDanger, cPrimary))
				} else {
					b.Background(cSelRow)
				}
				b.Children(func() {
					ui.Text(c, name).FontSize(pick[float32](l.compact, 19, 18)).Bold().TextColor(pick(sel, cWhite, cText))
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
	cpt := l.compact
	pad := pick[float32](cpt, 36, 56) // the panel's padding
	kw := min(64, (w-2*pad-8-9*7)/10) // ten keys and their gaps inside the panel
	kh := pick[float32](cpt, 40, 46)
	l.overlay(c, func() {
		ui.Text(c, k.title).FontSize(21).Bold().TextColor(cText)
		masked := ""
		for range k.text {
			masked += "•"
		}
		if len(k.text) > 0 {
			masked = masked[:len(masked)-len("•")] + k.text[len(k.text)-1:] // the last character shows
		}
		ui.Box(c).Width(kw*10+63).Padding(pick[float32](cpt, 9, 12)).Radius(14).Background(ui.RGBA(0, 0, 0, 0.30)).Border(1.5, cPrimary).Children(func() {
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
						key := ui.Box(c).Width(kwid).Height(kh).Radius(13).AlignItems(ui.Center).Justify(ui.Center)
						if sel {
							l.accent(key, cPrimary)
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
		ui.Text(c, "A 输入    B 删除    X 大小写    + 完成    − 取消").FontSize(pick[float32](cpt, 15, 14)).TextColor(cSecondary)
	})
}
