// Package study contains the local interactive study screens.
package study

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/progress"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/sponsor"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

const (
	maxFrame  = 104
	minWidth  = 40
	minHeight = 12
)

type Options struct {
	Progress *progress.Store
	Sponsor  sponsor.Sponsor
	Now      func() time.Time
	Renderer *lipgloss.Renderer
}

type styles struct {
	accent lipgloss.Style
	title  lipgloss.Style
	bold   lipgloss.Style
	muted  lipgloss.Style
	rule   lipgloss.Style
	good   lipgloss.Style
	bad    lipgloss.Style
	key    lipgloss.Style
	code   lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) styles {
	if r == nil {
		r = lipgloss.DefaultRenderer()
	}
	return styles{
		accent: r.NewStyle().Foreground(lipgloss.Color("#F6A823")),
		title:  r.NewStyle().Foreground(lipgloss.Color("#F6A823")).Bold(true),
		bold:   r.NewStyle().Bold(true),
		muted:  r.NewStyle().Foreground(lipgloss.Color("#A1A1AA")),
		rule:   r.NewStyle().Foreground(lipgloss.Color("#52525B")),
		good:   r.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true),
		bad:    r.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true),
		key:    r.NewStyle().Foreground(lipgloss.Color("#F6A823")).Bold(true),
		code:   r.NewStyle().Foreground(lipgloss.Color("#A1A1AA")),
	}
}

func now(fn func() time.Time) time.Time {
	if fn == nil {
		return time.Now()
	}
	return fn()
}

func size(w, h int) (int, int) {
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return w, h
}

func tooSmall(w, h int) bool { return w < minWidth || h < minHeight }

func frameWidth(w int) int {
	if w < maxFrame {
		return w
	}
	return maxFrame
}

func renderScreen(w, h, scroll int, title, sub string, sponsor sponsor.Sponsor, st styles, body []string, help string) string {
	w, h = size(w, h)
	if tooSmall(w, h) {
		message := fmt.Sprintf("This window is %dx%d. Please make it at least %dx%d, or press q to quit.", w, h, minWidth, minHeight)
		return fitLines(tui.Wrap(message, max(10, w-2)), w, h, w)
	}
	fw := frameWidth(w)
	tw := fw - 4
	header := []string{
		"  " + st.accent.Render(">_ ") + st.bold.Render(title),
		"  " + st.rule.Render(strings.Repeat("─", tw)),
		"  " + st.muted.Render(sub),
	}
	footer := []string{}
	if !sponsor.Empty() {
		line := st.muted.Render("Supported by ") + st.title.Render(sponsor.Name)
		if sponsor.Text != "" {
			line += st.muted.Render(": " + sponsor.Text)
		}
		footer = append(footer, "  "+line)
	}
	footer = append(footer, "  "+help)
	bodyHeight := max(1, h-len(header)-len(footer))
	if scroll < 0 {
		scroll = 0
	}
	if scroll > max(0, len(body)-bodyHeight) {
		scroll = max(0, len(body)-bodyHeight)
	}
	more := scroll+bodyHeight < len(body)
	if more {
		footer[len(footer)-1] += "   " + st.accent.Render("↓ more")
	}
	view := append([]string{}, header...)
	for _, line := range body[scroll:] {
		if len(view) >= len(header)+bodyHeight {
			break
		}
		view = append(view, "  "+line)
	}
	for len(view) < len(header)+bodyHeight {
		view = append(view, "")
	}
	view = append(view, footer...)
	return fitLines(view, fw, h, w)
}

func fitLines(lines []string, frame, height, window int) string {
	if len(lines) > height {
		lines = lines[:height]
	}
	pad := strings.Repeat(" ", max(0, (window-frame)/2))
	for i, line := range lines {
		lines[i] = pad + ansi.Truncate(line, frame, "")
	}
	return strings.Join(lines, "\n")
}

func help(st styles, items ...[2]string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = st.key.Render(item[0]) + " " + st.muted.Render(item[1])
	}
	return strings.Join(parts, st.muted.Render(" · "))
}

func codeLines(code string, width int, st styles) []string {
	if code == "" {
		return nil
	}
	lines := []string{st.rule.Render("┌" + strings.Repeat("─", max(1, width-2)) + "┐")}
	for _, line := range tui.HardWrap(code, max(8, width-4)) {
		lines = append(lines, st.rule.Render("│ ")+st.code.Render(tui.PadRight(line, max(1, width-4)))+st.rule.Render(" │"))
	}
	lines = append(lines, st.rule.Render("└"+strings.Repeat("─", max(1, width-2))+"┘"))
	return lines
}

func tuiLines(value string, width int, st lipgloss.Style) []string {
	lines := tui.Wrap(value, width)
	for i, line := range lines {
		if line != "" {
			lines[i] = st.Render(line)
		}
	}
	return lines
}

func scrollKey(scroll, bodyLen, bodyHeight int, key string) int {
	page := max(1, bodyHeight-1)
	switch key {
	case "up", "k":
		scroll--
	case "down", "j":
		scroll++
	case "pgup", "ctrl+b", "ctrl+u":
		scroll -= page
	case "pgdown", "ctrl+f", "ctrl+d":
		scroll += page
	case "home", "g":
		scroll = 0
	case "end", "G":
		scroll = bodyLen
	}
	if scroll < 0 {
		return 0
	}
	return scroll
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
