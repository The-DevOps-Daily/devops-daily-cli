package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/progress"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/study"
)

func (a *App) flashcardsCmd() *cobra.Command {
	var newLimit int
	var all bool
	cmd := &cobra.Command{
		Use:     "flashcards [deck]",
		Short:   "Study a flashcard deck with spaced repetition",
		Example: "  devops-daily flashcards docker",
		Args:    maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			decks, err := a.Store.Decks()
			if err != nil {
				return err
			}
			store, err := a.progressStore()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				a.printDecks(decks, store)
				return nil
			}
			if !a.IsTTY() {
				return fmt.Errorf("flashcards needs an interactive terminal")
			}
			deck, err := studyMatches(decks, func(value catalog.Deck) studyMatch {
				return studyMatch{ID: value.ID, Title: value.Title, Category: value.Category}
			}, args[0], "", "deck")
			if err != nil {
				return err
			}
			sp, err := a.studySponsor()
			if err != nil {
				return err
			}
			return a.RunTUI(study.NewFlashcards(deck, study.FlashOptions{Options: study.Options{Progress: store, Sponsor: sp, Now: a.Now, Renderer: a.renderer()}, NewLimit: newLimit, All: all}))
		}}
	cmd.Flags().IntVar(&newLimit, "new", 20, "new cards to introduce")
	cmd.Flags().BoolVar(&all, "all", false, "study every card")
	return cmd
}

func (a *App) printDecks(decks []catalog.Deck, store *progress.Store) {
	today := a.Now()
	todayText := today.UTC().Format("2006-01-02")
	rows := make([][3]string, 0, len(decks))
	for _, deck := range decks {
		state := store.Cards(deck.ID)
		due := 0
		fresh := 0
		for _, card := range deck.Cards {
			value := state[card.ID]
			if value.Box == 0 {
				fresh++
			} else if value.Due != "" && value.Due <= todayText {
				due++
			}
		}
		rows = append(rows, [3]string{deck.ID, deck.Title, fmt.Sprintf("%d cards, %d due, %d new", len(deck.Cards), due, fresh)})
	}
	a.printRows(rows)
	fmt.Fprintln(a.Stdout, a.muted("Study one: devops-daily flashcards <deck>"))
}
