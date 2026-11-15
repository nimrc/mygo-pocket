package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

var (
	cBg       = ui.Hex("#070b14")
	cPanel    = ui.Hex("#0f172a")
	cRow      = ui.Hex("#131c2e")
	cRowSel   = ui.Hex("#1e2b45")
	cAccent   = ui.Hex("#38bdf8")
	cText     = ui.Hex("#f1f5f9")
	cMuted    = ui.Hex("#8b9ab3")
	cFaint    = ui.Hex("#475569")
	cOn       = ui.Hex("#22c55e")
	cOff      = ui.Hex("#334155")
	cDanger   = ui.Hex("#f87171")
	cWarn     = ui.Hex("#fbbf24")
	weekdays  = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}
)

func pick[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func (l *launcher) view(c *ui.Context) {
	if l.note != "" && time.Since(l.noteAt) > 4*time.Second {
		l.note = ""
	} else if l.note != "" {
		c.After(time.Second)
	}
	l.slideBrightness, l.slideVolume = float64(l.s.brightness), float64(l.s.volume)
	ui.Box(c).Fill().Background(cBg).Children(func() {
		ui.Column(c).Fill().PaddingX(44).Padding(28).Gap(26).Children(func() {
			l.topBar(c)
			if l.focus == fGames {
				l.gamesView(c)
			} else {
				l.settingsView(c)
			}
			l.footer(c)
		})
		if l.running != "" {
			l.overlay(c, func() {
				ui.Text(c, "正在运行").FontSize(26).TextColor(cMuted)
				ui.Text(c, l.running).FontSize(44).Bold().TextColor(cText)
			})
		} else if l.modal != nil {
			l.modal.view(l, c)
		}
	})
}

// ---- top bar: the clock, the two screens, the status icons ----

func (l *launcher) topBar(c *ui.Context) {
	s := &l.s
	ui.Row(c).AlignItems(ui.Center).Children(func() {
		ui.Row(c).AlignItems(ui.Baseline).Gap(16).Width(520).Children(func() {
			ui.Text(c, s.now.Format("15:04")).FontSize(46).Bold().TextColor(cText)
			ui.Text(c, fmt.Sprintf("%d月%d日 %s", s.now.Month(), s.now.Day(), weekdays[s.now.Weekday()])).FontSize(22).TextColor(cMuted)
		})
		ui.Box(c).Grow(1)
		ui.Row(c).Padding(6).Radius(30).Background(cPanel).Gap(6).Children(func() {
			for i, name := range []string{"游戏", "设置"} {
				active := (i == 0) == (l.focus == fGames)
				tab := ui.Box(c).PaddingX(34).Padding(12).Radius(24).Background(pick(active, cAccent, ui.Transparent)).Children(func() {
					ui.Text(c, name).FontSize(24).Bold().TextColor(pick(active, cBg, cMuted))
				})
				if tab.Clicked() {
					l.focus = pick(i == 0, fGames, fCategories)
				}
			}
		})
		ui.Box(c).Grow(1)
		ui.Row(c).AlignItems(ui.Center).Gap(22).Width(520).Justify(ui.End).Children(func() {
			if s.bt {
				ui.Text(c, "◈").FontSize(24).TextColor(cText)
			}
			wifiIcon(c, s.wifi.enabled && s.wifi.ssid != "", s.signal)
			if s.muted {
				ui.Text(c, "静音").FontSize(20).TextColor(cMuted)
			}
			batteryIcon(c, s.bat)
			ui.Text(c, fmt.Sprintf("%d%%", s.bat.percent)).FontSize(24).TextColor(cText)
		})
	})
}

// wifiIcon draws four bars, as many lit as the signal is strong
func wifiIcon(c *ui.Context, connected bool, signal int) {
	lit := 0
	if connected {
		lit = clamp((signal+24)/25, 1, 4)
	}
	ui.Row(c).AlignItems(ui.End).Gap(3).Height(24).Children(func() {
		for i := 0; i < 4; i++ {
			ui.Box(c).Width(6).Height(float32(6+i*6)).Radius(2).Background(pick(i < lit, cText, cFaint))
		}
	})
}

// batteryIcon draws the cell, filled to its charge: green charging, amber under 20%, red under 10%
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
	ui.Row(c).AlignItems(ui.Center).Gap(2).Children(func() {
		ui.Box(c).Width(46).Height(24).Radius(6).Border(2, cMuted).Padding(3).Children(func() {
			ui.Box(c).Width(float32(36*clamp(b.percent, 0, 100)/100)).Height(14).Radius(3).Background(fill)
		})
		ui.Box(c).Width(3).Height(10).Radius(1).Background(cMuted)
	})
}

// ---- games: one card each, its colours, its title ----

func (l *launcher) gamesView(c *ui.Context) {
	ui.Row(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(36).Children(func() {
		for i, g := range games {
			sel := i == l.card
			card := ui.Column(c).Width(540).Height(pick[float32](sel, 380, 340)).Radius(28).Padding(34).Gap(8).Justify(ui.End).
				Gradient(g.from, g.to, 160).Border(pick[float32](sel, 4, 0), cText).Opacity(pick[float32](sel, 1, 0.72)).Children(func() {
				ui.Text(c, g.tag).FontSize(17).Bold().TextColor(ui.RGBA(255, 255, 255, 0.6))
				ui.Box(c).Grow(1)
				ui.Text(c, g.title).FontSize(42).Bold().TextColor(cText)
				ui.Text(c, g.sub).FontSize(22).TextColor(ui.RGBA(255, 255, 255, 0.7))
			})
			if card.Clicked() {
				if sel {
					l.launch(g)
				}
				l.card = i
			}
		}
	})
}

// ---- settings: the categories on the left, the chosen one's rows on the right ----

func (l *launcher) settingsView(c *ui.Context) {
	cat := categories[l.cat]
	rows := cat.rows(l)
	if l.focus == fRows {
		rows = l.anchor(rows)
	}
	ui.Row(c).Grow(1).Gap(28).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(300).Padding(14).Radius(24).Background(cPanel).Gap(6).Children(func() {
			for i, k := range categories {
				sel := i == l.cat
				item := ui.Row(c).AlignItems(ui.Center).Gap(16).PaddingX(20).Padding(16).Radius(16).
					Background(pick(sel, pick(l.focus == fCategories, cAccent, cRowSel), ui.Transparent)).Children(func() {
					ui.Text(c, k.icon).FontSize(24).Width(28).TextColor(pick(sel && l.focus == fCategories, cBg, cMuted))
					ui.Text(c, k.name).FontSize(26).TextColor(pick(sel && l.focus == fCategories, cBg, pick(sel, cText, cMuted)))
				})
				if item.Clicked() {
					l.cat, l.sel, l.selKey, l.focus = i, 0, "", fCategories
				}
			}
		})
		ui.Column(c).Grow(1).Gap(14).Children(func() {
			ui.Text(c, cat.name).FontSize(34).Bold().TextColor(cText)
			ui.Scroll(c).Grow(1).Gap(10).Children(func() {
				for i, r := range rows {
					l.rowView(c, r, i)
				}
			})
		})
	})
}

func (l *launcher) rowView(c *ui.Context, r row, i int) {
	if r.kind == kHeader {
		ui.Text(c, r.label).FontSize(20).Bold().TextColor(cAccent).Padding(4).Margin(4)
		return
	}
	sel := l.focus == fRows && i == l.sel
	el := ui.Row(c).AlignItems(ui.Center).Gap(18).PaddingX(26).Padding(18).Radius(18).Background(pick(sel, cRowSel, cRow)).
		Border(pick[float32](sel, 2, 0), cAccent).Children(func() {
		ui.Text(c, r.label).FontSize(26).TextColor(pick(r.danger, cDanger, cText)).Grow(1)
		switch r.kind {
		case kInfo, kAction:
			if r.value != "" {
				ui.Text(c, r.value).FontSize(22).TextColor(cMuted)
			}
			if r.kind == kAction {
				ui.Text(c, "›").FontSize(30).TextColor(cFaint)
			}
		case kToggle:
			switchView(c, r.on)
		case kChoice:
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				if ui.Text(c, "‹").FontSize(30).TextColor(pick(sel, cAccent, cFaint)).Padding(6).Clicked() {
					l.sel, l.selKey = i, r.id()
					r.adjust(-1)
				}
				ui.Text(c, r.value).FontSize(24).TextColor(cText).Width(220).TextAlign(ui.Center)
				if ui.Text(c, "›").FontSize(30).TextColor(pick(sel, cAccent, cFaint)).Padding(6).Clicked() {
					l.sel, l.selKey = i, r.id()
					r.adjust(1)
				}
			})
		case kSlider:
			if ui.Slider(c, r.slider, 0, 100).Width(560).Changed() {
				l.sel, l.selKey = i, r.id()
				r.set(int(*r.slider + 0.5))
			}
			ui.Text(c, r.value).FontSize(24).TextColor(cText).Width(80).TextAlign(ui.End)
		}
		if sel && r.alt != nil {
			ui.Text(c, "X "+r.altLabel).FontSize(20).TextColor(cMuted)
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

// switchView draws an on/off switch
func switchView(c *ui.Context, on bool) {
	ui.Row(c).Width(72).Height(40).Radius(20).Padding(4).Background(pick(on, cOn, cOff)).Justify(pick(on, ui.End, ui.Start)).Children(func() {
		ui.Box(c).Width(32).Height(32).Radius(16).Background(cText)
	})
}

// ---- footer: what the buttons do here, or the last action ----

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
	ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Box(c).Width(8).Height(8).Radius(4).Background(pick(l.busy != "", cWarn, pick(l.note != "", cAccent, cFaint)))
		ui.Text(c, hint).FontSize(20).TextColor(cMuted)
	})
}

// overlay dims everything and shows a panel in the middle
func (l *launcher) overlay(c *ui.Context, content func()) {
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(ui.RGBA(2, 6, 14, 0.82)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Padding(44).Radius(28).Background(cPanel).Gap(22).AlignItems(ui.Center).Children(content)
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
		ui.Text(c, m.question).FontSize(36).Bold().TextColor(cText)
		ui.Row(c).Gap(20).Children(func() {
			for i, name := range []string{"取消", "确认"} {
				b := ui.Box(c).Width(220).PaddingY(18).Radius(18).Background(pick(i == m.choice, pick(i == 1, cDanger, cAccent), cRow)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, name).FontSize(26).Bold().TextColor(pick(i == m.choice, cBg, cText))
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
	l.overlay(c, func() {
		ui.Text(c, k.title).FontSize(30).Bold().TextColor(cText)
		masked := ""
		for range k.text {
			masked += "•"
		}
		if len(k.text) > 0 {
			masked = masked[:len(masked)-len("•")] + k.text[len(k.text)-1:] // the last character shows
		}
		ui.Box(c).Width(1000).Padding(18).Radius(14).Background(cBg).Border(2, cAccent).Children(func() {
			ui.Text(c, masked+"▏").FontSize(30).TextColor(cText)
		})
		ui.Column(c).Gap(10).Children(func() {
			for r, keys := range keyRows {
				ui.Row(c).Gap(10).Justify(ui.Center).Children(func() {
					for col := range keys {
						sel := r == k.row && col == k.col
						w := float32(88)
						if r == len(keyRows)-1 {
							w = 240
						}
						key := ui.Box(c).Width(w).Height(72).Radius(14).Background(pick(sel, cAccent, cRow)).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
							ui.Text(c, k.key(r, col)).FontSize(28).TextColor(pick(sel, cBg, cText))
						})
						if key.Clicked() {
							k.row, k.col = r, col
							k.typeKey(l, r, col)
						}
					}
				})
			}
		})
		ui.Text(c, "A 输入    B 删除    X 大小写    + 完成    − 取消").FontSize(20).TextColor(cMuted)
	})
}
