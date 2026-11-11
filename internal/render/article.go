package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

func Article(meta catalog.PostMeta, body string, width int, style string) (string, error) {
	rendered, err := Markdown(body, width, style)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	if style == "notty" {
		output.WriteString(meta.Title)
	} else {
		renderer := lipgloss.NewRenderer(io.Discard)
		renderer.SetColorProfile(termenv.TrueColor)
		output.WriteString(renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("#F6A823")).Render(meta.Title))
	}
	metaLine := strings.Join(nonEmpty(meta.Date, meta.ReadingTime, meta.Category, strings.Join(meta.Tags, ", ")), " · ")
	if metaLine != "" {
		output.WriteByte('\n')
		if style == "notty" {
			output.WriteString(metaLine)
		} else {
			output.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#A1A1AA")).Render(metaLine))
		}
	}
	output.WriteString("\n\n")
	output.WriteString(trimRendered(rendered))
	if links := Links(body, meta.URL); len(links) > 0 {
		output.WriteString("\n\n")
		if style == "notty" {
			output.WriteString("Links")
		} else {
			output.WriteString(lipgloss.NewStyle().Bold(true).Render("Links"))
		}
		output.WriteByte('\n')
		for i, link := range links {
			fmt.Fprintf(&output, "[%d] %s  %s\n", i+1, link.Text, link.URL)
		}
	}
	if meta.URL != "" {
		fmt.Fprintf(&output, "\nRead on the site: %s", meta.URL)
	}
	return output.String(), nil
}

func nonEmpty(values ...string) []string {
	var result []string
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}
