package cli

import (
	"fmt"
	"strings"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

// printRows prints one row per item: the id padded to a column, the title cut
// to fit the width, and a muted note in its own column.
func (a *App) printRows(rows [][3]string) {
	idWidth, noteWidth := 0, 0
	for _, row := range rows {
		idWidth = max(idWidth, min(len(row[0]), 44))
		noteWidth = max(noteWidth, len(row[2]))
	}
	titleWidth := a.width() - idWidth - noteWidth - 4
	for _, row := range rows {
		if titleWidth < 12 {
			fmt.Fprintf(a.Stdout, "%s  %s\n  %s\n", row[0], a.muted(row[2]), truncate(row[1], a.width()-2))
			continue
		}
		title := tui.PadRight(truncate(row[1], titleWidth), titleWidth)
		fmt.Fprintf(a.Stdout, "%s  %s  %s\n", tui.PadRight(row[0], idWidth), title, a.muted(row[2]))
	}
}

func (a *App) width() int {
	width := a.Width()
	if width <= 0 {
		return 80
	}
	return width
}
func truncate(value string, width int) string { return text.Truncate(value, width) }
func slug(value string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(value) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			out.WriteRune(r)
			dash = false
		} else if !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(out.String(), "-")
}
func alias(value string) string {
	switch value {
	case "k8s":
		return "kubernetes"
	case "tf":
		return "terraform"
	case "gha":
		return "github-actions"
	case "pg", "postgres":
		return "postgresql"
	case "cicd":
		return "ci-cd"
	}
	return value
}
