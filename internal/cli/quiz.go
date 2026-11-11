package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/progress"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/study"
)

func (a *App) quizCmd() *cobra.Command {
	var count int
	var shuffle bool
	cmd := &cobra.Command{
		Use:     "quiz [topic]",
		Short:   "Play a quiz from the site",
		Example: "  devops-daily quiz kubernetes",
		Args:    maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			quizzes, err := a.Store.Quizzes()
			if err != nil {
				return err
			}
			store, err := a.progressStore()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				a.printQuizzes(quizzes, store)
				return nil
			}
			if !a.IsTTY() {
				return fmt.Errorf("quiz needs an interactive terminal")
			}
			quiz, err := studyMatches(quizzes, func(value catalog.Quiz) studyMatch {
				return studyMatch{ID: value.ID, Title: value.Title, Category: value.Category}
			}, args[0], "-quiz", "quiz")
			if err != nil {
				return err
			}
			sp, err := a.studySponsor()
			if err != nil {
				return err
			}
			return a.RunTUI(study.NewQuiz(quiz, study.QuizOptions{Options: study.Options{Progress: store, Sponsor: sp, Now: a.Now, Renderer: a.renderer()}, Count: count, Shuffle: shuffle, Seed: a.Now().UnixNano()}))
		}}
	cmd.Flags().IntVar(&count, "count", 0, "questions to ask")
	cmd.Flags().BoolVar(&shuffle, "shuffle", false, "shuffle questions")
	return cmd
}

func (a *App) printQuizzes(quizzes []catalog.Quiz, store *progress.Store) {
	rows := make([][3]string, 0, len(quizzes))
	for _, quiz := range quizzes {
		note := fmt.Sprintf("%d questions", len(quiz.Questions))
		if score, ok := store.Quiz(quiz.ID); ok {
			note += fmt.Sprintf(", best %d/%d", score.Best, score.Total)
		}
		rows = append(rows, [3]string{quiz.ID, quiz.Title, note})
	}
	a.printRows(rows)
	fmt.Fprintln(a.Stdout, a.muted("Play one: devops-daily quiz <topic>"))
}
