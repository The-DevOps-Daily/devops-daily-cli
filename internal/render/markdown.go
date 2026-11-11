package render

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/glamour"
)

// glamour pads every line to the wrap width; the padding only gets in the
// way when the text is copied or piped.
var trailingSpace = regexp.MustCompile(`(?m) +((?:\x1b\[[0-9;]*m)*)$`)

func Markdown(markdown string, width int, style string) (string, error) {
	width = max(20, min(100, width))
	if style == "auto" {
		style = "dark"
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(width),
		glamour.WithStandardStyle(style),
	)
	if err != nil {
		return "", err
	}
	out, err := renderer.Render(markdown)
	if err != nil {
		return "", err
	}
	return trailingSpace.ReplaceAllString(out, "$1"), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func trimRendered(value string) string {
	return strings.TrimRight(value, "\n")
}
