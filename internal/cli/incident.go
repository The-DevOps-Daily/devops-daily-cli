package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/content"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

func (a *App) incidentCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:     "incident",
		Short:   "Show today's incident",
		Example: "  devops-daily incident --fix",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			deps, err := a.homeDeps()
			if err != nil {
				return err
			}
			incident := deps.Content.IncidentFor(a.Now())
			if a.IsTTY() {
				deps.Start = "incident"
				return a.RunTUI(tui.New(a.renderer(), deps))
			}
			a.printIncident(incident, fix)
			return nil
		}}
	cmd.Flags().BoolVar(&fix, "fix", false, "show the fix")
	return cmd
}
func (a *App) printIncident(incident content.Incident, fix bool) {
	fmt.Fprintln(a.Stdout, incident.Title)
	meta := incident.Difficulty
	if len(incident.Technologies) > 0 {
		if meta != "" {
			meta += " · "
		}
		meta += strings.Join(incident.Technologies, ", ")
	}
	if meta != "" {
		fmt.Fprintln(a.Stdout, a.muted(meta))
	}
	fmt.Fprintln(a.Stdout)
	for _, line := range wrapText(incident.Scenario, a.width()) {
		fmt.Fprintln(a.Stdout, line)
	}
	for _, option := range incident.Options {
		fmt.Fprintf(a.Stdout, "%s) %s\n", option.Key, option.Text)
	}
	if !fix {
		fmt.Fprintln(a.Stdout, "Run with --fix to see the answer.")
		return
	}
	correct := incident.CorrectOption()
	fmt.Fprintf(a.Stdout, "\nThe fix: %s) %s\n%s\nLesson: %s\nGo deeper: %s\n%s\n", correct.Key, correct.Text, incident.Fix, incident.Lesson, incident.LinkTitle, incident.LinkURL)
}
func wrapText(value string, width int) []string {
	if width < 20 {
		width = 20
	}
	words := strings.Fields(value)
	var lines []string
	line := ""
	for _, word := range words {
		if len(line)+len(word)+1 > width && line != "" {
			lines = append(lines, line)
			line = word
		} else if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
