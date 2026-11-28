package gui

import "github.com/egoist/mygo/ui"

// Design tokens for package gui. Colors, typography/spacing scales, and SVG
// icons live here; nothing else in the package names a raw color.

const (
	fontHero    float32 = 17
	fontTitle   float32 = 14
	fontHeading float32 = 12
	fontBody    float32 = 11
	fontCaption float32 = 10
	fontMicro   float32 = 9.5

	radiusCard    float32 = 12
	radiusSub     float32 = 9
	radiusControl float32 = 7
	radiusSmall   float32 = 5
	radiusPill    float32 = 999

	spaceXS float32 = 4
	spaceSM float32 = 8
	spaceMD float32 = 12
	spaceLG float32 = 16
)

type palette struct {
	dark       bool
	bg         ui.Color
	card       ui.Color
	cardSub    ui.Color
	soft       ui.Color
	fg         ui.Color
	muted      ui.Color
	line       ui.Color
	accent     ui.Color
	accentSoft ui.Color
	onAccent   ui.Color
	ok         ui.Color
	okSoft     ui.Color
	danger     ui.Color
	dangerSoft ui.Color
	track      ui.Color
	fresh      ui.Color
	cache      ui.Color
	output     ui.Color
	pillBg     ui.Color
	pillText   ui.Color
	warnBg     ui.Color
	warnText   ui.Color
	highBg     ui.Color
}

func currentPalette(c *ui.Context) palette {
	dark := c.Theme().Background.R < 128
	if dark {
		return palette{
			dark:       true,
			bg:         ui.RGB(24, 26, 32),
			card:       ui.RGBA(40, 44, 54, 0.92),
			cardSub:    ui.RGBA(28, 31, 38, 0.90),
			soft:       ui.RGBA(255, 255, 255, 0.08),
			fg:         ui.RGB(240, 243, 249),
			muted:      ui.RGB(154, 163, 178),
			line:       ui.RGBA(255, 255, 255, 0.11),
			accent:     ui.RGB(59, 130, 246),
			accentSoft: ui.RGBA(59, 130, 246, 0.22),
			onAccent:   ui.RGB(255, 255, 255),
			ok:         ui.RGB(52, 211, 153),
			okSoft:     ui.RGBA(52, 211, 153, 0.20),
			danger:     ui.RGB(248, 113, 113),
			dangerSoft: ui.RGBA(248, 113, 113, 0.16),
			track:      ui.RGBA(255, 255, 255, 0.12),
			fresh:      ui.RGB(139, 180, 255),
			cache:      ui.RGB(197, 163, 255),
			output:     ui.RGB(110, 231, 183),
			pillBg:     ui.RGBA(255, 255, 255, 0.14),
			pillText:   ui.RGB(240, 243, 249),
			warnBg:     ui.RGBA(251, 191, 36, 0.18),
			warnText:   ui.RGB(251, 191, 36),
			highBg:     ui.RGBA(248, 113, 113, 0.20),
		}
	}
	return palette{
		bg:         ui.RGB(244, 246, 251),
		card:       ui.RGBA(255, 255, 255, 0.96),
		cardSub:    ui.RGB(248, 250, 253),
		soft:       ui.RGBA(15, 23, 42, 0.06),
		fg:         ui.RGB(18, 24, 38),
		muted:      ui.RGB(100, 116, 139),
		line:       ui.RGBA(15, 23, 42, 0.10),
		accent:     ui.RGB(37, 99, 235),
		accentSoft: ui.RGBA(37, 99, 235, 0.12),
		onAccent:   ui.RGB(255, 255, 255),
		ok:         ui.RGB(16, 185, 129),
		okSoft:     ui.RGBA(16, 185, 129, 0.20),
		danger:     ui.RGB(220, 38, 38),
		dangerSoft: ui.RGBA(220, 38, 38, 0.10),
		track:      ui.RGBA(15, 23, 42, 0.10),
		fresh:      ui.RGB(139, 180, 255),
		cache:      ui.RGB(197, 163, 255),
		output:     ui.RGB(110, 231, 183),
		pillBg:     ui.RGB(255, 255, 255),
		pillText:   ui.RGB(18, 24, 38),
		warnBg:     ui.RGBA(245, 158, 11, 0.16),
		warnText:   ui.RGB(180, 83, 9),
		highBg:     ui.RGBA(220, 38, 38, 0.14),
	}
}

func (p palette) quotaColor(pct int) ui.Color {
	if pct >= 50 {
		return p.ok
	}
	if pct >= 20 {
		return p.warnText
	}
	return p.danger
}

var (
	sunSVG            = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>`))
	moonSVG           = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="currentColor"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>`))
	chevronRightSVG   = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`))
	chevronDownSVG    = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`))
	chevronsUpDownSVG = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/></svg>`))
	refreshSVG        = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-2.64-6.36"/><path d="M21 3v6h-6"/></svg>`))
	clockSVG          = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`))
	plusSVG           = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 5v14"/><path d="M5 12h14"/></svg>`))
	copySVG           = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>`))
	closeSVG          = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`))
	accountsSVG       = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>`))
	projectsSVG       = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>`))
	settingsSVG       = ui.MustParseSVG([]byte(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z"/></svg>`))
)
