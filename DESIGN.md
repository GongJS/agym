# Agy Manager — Design System (`DESIGN.md`)

> Dual-theme native desktop design system for **Agy Manager** (`700×520` pure-Go MyGo Native UI):
> - **Dark (Default):** *AuthKit — Frosted glass cathedral at midnight*
> - **Light:** *Calendly — Navy ink on cool marble*

Both themes share **one geometry, typography, and interaction skeleton** so switching between Dark and Light never shifts layout, spacing, or border radii.

---

## 1. Semantic Color Tokens

All UI code in `internal/gui` must consume these semantic roles from [`internal/gui/theme.go`](internal/gui/theme.go). Raw color literals outside `theme.go` are prohibited.

| Semantic Token | Dark (`AuthKit`) | Light (`Calendly`) | Role |
|---|---|---|---|
| `bg` | `#05060f` | `#f8f9fb` | Main window & content canvas |
| `card` | `rgba(186, 214, 247, 0.04)` | `#ffffff` | Primary interactive cards & popovers |
| `cardHover` | `rgba(186, 214, 247, 0.08)` | `#f5f8fd` | Interactive card hover fill |
| `cardSub` | `rgba(12, 15, 28, 0.94)` | `#f0f3f8` | Left sidebar & nested sub-panels |
| `soft` | `rgba(186, 214, 247, 0.11)` | `#e5ebf4` | Secondary/ghost button & pill fill |
| `softHover` | `rgba(186, 214, 247, 0.18)` | `#d6dfed` | Secondary/ghost button hover fill |
| `ghostHover` | `rgba(186, 214, 247, 0.06)` | `rgba(11, 53, 88, 0.05)` | Transparent nav/icon button hover fill |
| `fg` | `#d1e4fa` | `#0b3558` | Primary body text, headings, icons |
| `muted` | `#9da7ba` | `#476788` | Secondary labels, helper copy, metadata |
| `line` | `rgba(186, 215, 247, 0.12)` | `#d4e0ed` | 1px hairline borders & dividers |
| `lineHover` | `rgba(186, 215, 247, 0.24)` | `#a6bbd1` | Hovered / focused hairline border |
| `accent` | `#663af3` | `#006bff` | Primary CTA fill & active selection |
| `accentHover` | `#764ef5` | `#005ae0` | Primary CTA hover fill |
| `accentSoft` | `rgba(102, 58, 243, 0.22)` | `#e6f0ff` | Active nav item, selected model/badge fill |
| `accentSoftHover` | `rgba(102, 58, 243, 0.34)` | `#d4e5ff` | Soft accent button hover fill |
| `onAccent` | `#ffffff` | `#ffffff` | Text/icons on solid `accent` or `danger` fills |
| `ok` | `#269684` | `#10b981` | Healthy quota (>=50%) & active status dot |
| `okSoft` | `rgba(38, 150, 132, 0.20)` | `rgba(16, 185, 129, 0.14)` | Active status halo |
| `warnText` | `#fbbf24` | `#b45309` | Mid quota (20–49%), stale/need-login badge text |
| `warnBg` | `rgba(251, 191, 36, 0.16)` | `rgba(245, 158, 11, 0.14)` | Stale/warning banner & badge fill |
| `danger` | `#e46d4c` | `#dc2626` | Low quota (<20%), destructive action, error |
| `dangerHover` | `#ee7e5f` | `#be1c1c` | Solid destructive button hover |
| `dangerSoft` | `rgba(228, 109, 76, 0.16)` | `rgba(220, 38, 38, 0.10)` | Destructive secondary button & error banner fill |
| `dangerSoftHover` | `rgba(228, 109, 76, 0.26)` | `rgba(220, 38, 38, 0.17)` | Destructive secondary button hover |
| `track` | `rgba(186, 215, 247, 0.12)` | `#d4e0ed` | Progress bar & dot-matrix empty track |
| `fresh` | `#b6d9fc` | `#006bff` | Project context matrix: fresh input |
| `cache` | `#027dea` | `#0099ff` | Project context matrix: cache read |
| `output` | `#269684` | `#10b981` | Project context matrix: model output |
| `pillBg` | `rgba(199, 211, 234, 0.14)` | `#ffffff` | Active segmented control pill |
| `pillText` | `#d8ecf8` | `#0b3558` | Active segmented control text |

---

## 2. Typography, Spacing & Radii (Shared)

- **Font Family:** System native UI sans (`SF Pro` on macOS, `Segoe UI` on Windows).
- **Type Scale (6 steps, floor `10px`):**
  - `fontHero`: `17px` (Context window hero metric)
  - `fontTitle`: `14px` (Section header title)
  - `fontHeading`: `12px` (Card & section headings)
  - `fontBody`: `11px` (Account emails, nav items, primary labels)
  - `fontCaption`: `10.5px` (Helper copy, quota rows, action buttons)
  - `fontMicro`: `10px` (Badges, reset countdowns, footer note)
- **Spacing Scale (`4px` base unit):**
  - `spaceXS`: `4px` · `spaceSM`: `8px` · `spaceMD`: `12px` · `spaceLG`: `16px`
- **Border Radius (5 steps):**
  - `radiusCard`: `16px` (Main account, project, settings, onboarding cards & popovers)
  - `radiusSub`: `10px` (Sub-cards, error banners, sidebar account card)
  - `radiusControl`: `6px` (Action buttons, nav items, text inputs, segmented controls)
  - `radiusSmall`: `4px` (Compact inline actions, matrix cells, legend swatches)
  - `radiusPill`: `999px` (Status badges, tier pills, avatars, progress bars)

---

## 3. Component & Interaction Rules

- **Do:**
  - Use `1px` `line` borders (`lineHover` on hover) for elevation — no heavy drop shadows.
  - Apply `hoverBg` / `hoverCard` to all enabled interactive controls; disabled controls remain muted with no hover shift.
  - Keep `accent` (`#663af3` dark / `#006bff` light) reserved for primary CTAs and active selection state.
  - Require inline two-step confirmation (`移除` → `取消 / 确认移除`) for destructive account/project removal.
- **Don't:**
  - Never use pure `#000000` for body text in Light mode (use Ink Navy `#0b3558` / Slate Gray `#476788`).
  - Never hardcode `RGB` / `RGBA` / `Hex` or magic `FontSize` / `Radius` numbers outside `internal/gui/theme.go`.
  - Never hide stale or failed quota status; surface `Status == "stale"` with `CheckedAt` age and non-`ok` failure messages.
