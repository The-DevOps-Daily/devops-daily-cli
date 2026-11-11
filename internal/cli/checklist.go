package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/progress"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/study"
)

func (a *App) checklistCmd() *cobra.Command {
	var reset bool
	var yes bool
	cmd := &cobra.Command{
		Use:     "checklist [name]",
		Short:   "Work through a checklist; progress is saved on this machine",
		Example: "  devops-daily checklist deployment",
		Args:    maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			checklists, err := a.Store.Checklists()
			if err != nil {
				return err
			}
			store, err := a.progressStore()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				a.printChecklists(checklists, store)
				return nil
			}
			checklist, err := studyMatches(checklists, func(value catalog.Checklist) studyMatch {
				return studyMatch{ID: value.ID, Title: value.Title, Category: value.Category}
			}, args[0], "-checklist", "checklist")
			if err != nil {
				return err
			}
			if reset {
				if !yes && !a.IsTTY() {
					return fmt.Errorf("--reset needs --yes without an interactive terminal")
				}
				if !yes {
					fmt.Fprintf(a.Stdout, "Reset all %d items? y/n ", len(checklist.Items))
					var answer string
					if _, err := fmt.Fscan(a.Stdin, &answer); err != nil || strings.ToLower(answer) != "y" {
						fmt.Fprintln(a.Stdout, "Not reset.")
						return nil
					}
				}
				return store.ResetChecklist(checklist.ID)
			}
			if !a.IsTTY() {
				var output strings.Builder
				plainChecklist(&output, checklist, store.ChecklistDone(checklist.ID))
				_, err := fmt.Fprint(a.Stdout, output.String())
				return err
			}
			sp, err := a.studySponsor()
			if err != nil {
				return err
			}
			return a.RunTUI(study.NewChecklist(checklist, study.ChecklistOptions{Options: study.Options{Progress: store, Sponsor: sp, Now: a.Now, Renderer: a.renderer()}}))
		}}
	cmd.Flags().BoolVar(&reset, "reset", false, "clear saved progress")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm reset")
	return cmd
}

func (a *App) printChecklists(checklists []catalog.Checklist, store *progress.Store) {
	rows := make([][3]string, 0, len(checklists))
	for _, checklist := range checklists {
		done := 0
		for _, item := range checklist.Items {
			if store.ChecklistDone(checklist.ID)[item.ID] {
				done++
			}
		}
		rows = append(rows, [3]string{checklist.ID, checklist.Title, fmt.Sprintf("%d/%d done", done, len(checklist.Items))})
	}
	a.printRows(rows)
	fmt.Fprintln(a.Stdout, a.muted("Open one: devops-daily checklist <name>"))
}
