package tui

import "github.com/charmbracelet/lipgloss"

// Colors follow the site's dark theme: the orange primary and zinc greys.
const (
	colorAccent = lipgloss.Color("#F6A823")
	colorMuted  = lipgloss.Color("#A1A1AA")
	colorRule   = lipgloss.Color("#52525B")
	colorGood   = lipgloss.Color("#22C55E")
	colorBad    = lipgloss.Color("#EF4444")
)

type styles struct {
	accent   lipgloss.Style
	title    lipgloss.Style
	bold     lipgloss.Style
	text     lipgloss.Style
	muted    lipgloss.Style
	rule     lipgloss.Style
	good     lipgloss.Style
	bad      lipgloss.Style
	selected lipgloss.Style
	code     lipgloss.Style
	key      lipgloss.Style
	box      lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) styles {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	return styles{
		accent:   r.NewStyle().Foreground(colorAccent),
		title:    r.NewStyle().Foreground(colorAccent).Bold(true),
		bold:     r.NewStyle().Bold(true),
		text:     r.NewStyle(),
		muted:    r.NewStyle().Foreground(colorMuted),
		rule:     r.NewStyle().Foreground(colorRule),
		good:     r.NewStyle().Foreground(colorGood).Bold(true),
		bad:      r.NewStyle().Foreground(colorBad).Bold(true),
		selected: r.NewStyle().Foreground(colorAccent).Bold(true),
		code:     r.NewStyle().Foreground(colorMuted),
		key:      r.NewStyle().Foreground(colorAccent).Bold(true),
		box:      r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent).Padding(0, 2),
	}
}
