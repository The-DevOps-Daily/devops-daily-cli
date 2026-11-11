package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "sync",
		Short:   "Download the latest content",
		Example: "  devops-daily sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := a.Store.Sync(cmd.Context())
			if err != nil {
				return err
			}
			if result.FilesDownloaded == 0 {
				fmt.Fprintln(a.Stdout, "Up to date.")
			} else {
				fmt.Fprintf(a.Stdout, "Downloaded %d files (%.1f MB).\n", result.FilesDownloaded, float64(result.BytesDownloaded)/(1024*1024))
			}
			manifest, err := a.Store.Manifest()
			if err != nil {
				return err
			}
			fmt.Fprintf(a.Stdout, "%d posts, %d quizzes, %d flashcard decks, %d checklists, %d incidents, %d games\nCache: %s\n", manifest.Counts["posts"], manifest.Counts["quizzes"], manifest.Counts["flashcards"], manifest.Counts["checklists"], manifest.Counts["incidents"], manifest.Counts["games"], a.Store.Dir)
			return nil
		}}
}
