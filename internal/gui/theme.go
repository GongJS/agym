package gui

import "github.com/egoist/mygo/ui"

// Design tokens for package gui. Colors, typography/spacing scales, and SVG
// icons live here; nothing else in the package names a raw color.

const (
	fontHero    float32 = 17
	fontTitle   float32 = 14
	fontHeading float32 = 12
	fontBody    float32 = 11
	fontCaption float32 = 10.5
	fontMicro   float32 = 10

	radiusCard    float32 = 16
	radiusSub     float32 = 10
	radiusControl float32 = 6
	radiusSmall   float32 = 4
	radiusPill    float32 = 999

	spaceXS float32 = 4
	spaceSM float32 = 8
	spaceMD float32 = 12
	spaceLG float32 = 16
)

type palette struct {
	dark            bool
	bg              ui.Color
	card            ui.Color
	cardHover       ui.Color
	cardSub         ui.Color
	soft            ui.Color
	softHover       ui.Color
	ghostHover      ui.Color
	fg              ui.Color
	muted           ui.Color
	line            ui.Color
	lineHover       ui.Color
	accent          ui.Color
	accentHover     ui.Color
	accentSoft      ui.Color
	accentSoftHover ui.Color
	onAccent        ui.Color
	ok              ui.Color
	okSoft          ui.Color
	danger          ui.Color
	dangerHover     ui.Color
	dangerSoft      ui.Color
	dangerSoftHover ui.Color
	track           ui.Color
	fresh           ui.Color
	cache           ui.Color
	output          ui.Color
	pillBg          ui.Color
	pillText        ui.Color
	warnBg          ui.Color
	warnText        ui.Color
	highBg          ui.Color
}

func currentPalette(c *ui.Context) palette {
	dark := c.Theme().Background.R < 128
	if dark {
		// AuthKit — "Frosted glass cathedral at midnight" (DESIGN.md / tokens.json)
		return palette{
			dark:            true,
			bg:              ui.RGB(5, 6, 15),             // #05060f Midnight Canvas
			card:            ui.RGBA(186, 214, 247, 0.04), // Frosted Glass
			cardHover:       ui.RGBA(186, 214, 247, 0.08), // Frosted Glass hover
			cardSub:         ui.RGBA(12, 15, 28, 0.94),    // Deep Glass sidebar / sub-surface
			soft:            ui.RGBA(186, 214, 247, 0.11), // Deepened frost wash for clearer button surface
			softHover:       ui.RGBA(186, 214, 247, 0.18), // Hover lightens frost wash to 0.18
			ghostHover:      ui.RGBA(186, 214, 247, 0.06), // Transparent control hover
			fg:              ui.RGB(209, 228, 250),        // #d1e4fa Frost Glow
			muted:           ui.RGB(157, 167, 186),        // #9da7ba Fog Veil
			line:            ui.RGBA(186, 215, 247, 0.12), // #bad7f71f Glass Edge
			lineHover:       ui.RGBA(186, 215, 247, 0.24), // Focus / hover border (DESIGN.md)
			accent:          ui.RGB(102, 58, 243),         // #663af3 Void Violet (CTA & active)
			accentHover:     ui.RGB(118, 78, 245),         // Void Violet hover
			accentSoft:      ui.RGBA(102, 58, 243, 0.22),  // Void Violet tint
			accentSoftHover: ui.RGBA(102, 58, 243, 0.34),
			onAccent:        ui.RGB(255, 255, 255),        // #ffffff Pure White
			ok:              ui.RGB(38, 150, 132),         // #269684 Deep Teal
			okSoft:          ui.RGBA(38, 150, 132, 0.20),
			danger:          ui.RGB(228, 109, 76),         // #e46d4c Ember Glow
			dangerHover:     ui.RGB(238, 126, 95),
			dangerSoft:      ui.RGBA(228, 109, 76, 0.16),
			dangerSoftHover: ui.RGBA(228, 109, 76, 0.26),
			track:           ui.RGBA(186, 215, 247, 0.12), // Glass Edge track
			fresh:           ui.RGB(182, 217, 252),        // #b6d9fc Blueprint Blue
			cache:           ui.RGB(2, 125, 234),          // #027dea Signal Blue
			output:          ui.RGB(38, 150, 132),         // #269684 Deep Teal
			pillBg:          ui.RGBA(199, 211, 234, 0.14), // Luminous active pill
			pillText:        ui.RGB(216, 236, 248),        // #d8ecf8 Ice Highlight
			warnBg:          ui.RGBA(251, 191, 36, 0.16),
			warnText:        ui.RGB(251, 191, 36),
			highBg:          ui.RGBA(228, 109, 76, 0.20),
		}
	}
	// Calendly — "Navy ink on cool marble" (DESIGN-1.md / tokens-1.json)
	return palette{
		bg:              ui.RGB(248, 249, 251),      // #f8f9fb Cloud
		card:            ui.RGB(255, 255, 255),      // #ffffff Paper
		cardHover:       ui.RGB(245, 248, 253),      // Subtle cool lift
		cardSub:         ui.RGB(240, 243, 248),      // #f0f3f8 Pebble sidebar & sub-surface
		soft:            ui.RGB(229, 235, 244),      // #e5ebf4 Deepened Pebble
		softHover:       ui.RGB(214, 223, 237),      // #d6dfed Hover Pebble
		ghostHover:      ui.RGBA(11, 53, 88, 0.05),  // Ink Navy wash
		fg:              ui.RGB(11, 53, 88),         // #0b3558 Ink Navy
		muted:           ui.RGB(71, 103, 136),       // #476788 Slate Gray
		line:            ui.RGB(212, 224, 237),      // #d4e0ed Hairline
		lineHover:       ui.RGB(166, 187, 209),      // #a6bbd1 Mist Gray
		accent:          ui.RGB(0, 107, 255),        // #006bff Signal Blue
		accentHover:     ui.RGB(0, 90, 224),
		accentSoft:      ui.RGB(230, 240, 255),      // #e6f0ff Pebble-tinted pill
		accentSoftHover: ui.RGB(212, 229, 255),
		onAccent:        ui.RGB(255, 255, 255),      // #ffffff Paper
		ok:              ui.RGB(16, 185, 129),
		okSoft:          ui.RGBA(16, 185, 129, 0.14),
		danger:          ui.RGB(220, 38, 38),
		dangerHover:     ui.RGB(190, 28, 28),
		dangerSoft:      ui.RGBA(220, 38, 38, 0.10),
		dangerSoftHover: ui.RGBA(220, 38, 38, 0.17),
		track:           ui.RGB(212, 224, 237),      // #d4e0ed Hairline
		fresh:           ui.RGB(0, 107, 255),        // #006bff Signal Blue
		cache:           ui.RGB(0, 153, 255),        // #0099ff Sky Cyan
		output:          ui.RGB(16, 185, 129),
		pillBg:          ui.RGB(255, 255, 255),      // #ffffff Paper
		pillText:        ui.RGB(11, 53, 88),         // #0b3558 Ink Navy
		warnBg:          ui.RGBA(245, 158, 11, 0.14),
		warnText:        ui.RGB(180, 83, 9),
		highBg:          ui.RGBA(220, 38, 38, 0.14),
	}
}

func hoverBg(el ui.Element, bg ui.Color) ui.Element {
	if el.Hovered() {
		return el.Background(bg)
	}
	return el
}

func hoverCard(el ui.Element, bg, border ui.Color) ui.Element {
	if el.Hovered() {
		return el.Background(bg).Border(1, border)
	}
	return el
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
