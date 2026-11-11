package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
)

func (a *App) playCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:     "play [name]",
		Short:   "Open a simulator or game",
		Example: "  devops-daily play dns",
		Args:    maximumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			games, err := a.Store.Games()
			if err != nil {
				return err
			}
			if len(args) == 0 {
				a.printGames(games)
				return nil
			}
			game, err := findGame(games, args[0])
			if err != nil {
				return err
			}
			if printOnly {
				_, err := fmt.Fprintln(a.Stdout, game.URL)
				return err
			}
			fmt.Fprintln(a.Stdout, "Opening "+game.URL)
			if err := a.OpenURL(game.URL); err != nil {
				fmt.Fprintln(a.Stdout, "Could not open a browser. Open this link:")
				fmt.Fprintln(a.Stdout, game.URL)
			}
			return nil
		}}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the URL only")
	return cmd
}

func (a *App) printGames(games []catalog.Game) {
	idWidth := 0
	for _, game := range games {
		idWidth = max(idWidth, min(len(game.ID), 40))
	}
	for i, kind := range []string{"simulator", "game"} {
		if i > 0 {
			fmt.Fprintln(a.Stdout)
		}
		heading := "Simulators"
		if kind == "game" {
			heading = "Games"
		}
		fmt.Fprintln(a.Stdout, a.bold(heading))
		for _, game := range games {
			if game.Type != kind {
				continue
			}
			id := game.ID
			if pad := idWidth - len(id); pad > 0 {
				id += strings.Repeat(" ", pad)
			}
			title := truncate(game.Title, max(10, a.width()-len(id)-4))
			fmt.Fprintf(a.Stdout, "  %s  %s\n", id, title)
		}
	}
	fmt.Fprintln(a.Stdout)
	fmt.Fprintln(a.Stdout, a.muted("Open one: devops-daily play <id>"))
}
func findGame(games []catalog.Game, want string) (catalog.Game, error) {
	want = strings.ToLower(want)
	for _, game := range games {
		if game.ID == want || game.ID == want+"-simulator" {
			return game, nil
		}
	}
	var prefixes []catalog.Game
	for _, game := range games {
		if strings.HasPrefix(game.ID, want) {
			prefixes = append(prefixes, game)
		}
	}
	if len(prefixes) == 1 {
		return prefixes[0], nil
	}
	if len(prefixes) > 1 {
		return catalog.Game{}, severalGames(want, prefixes)
	}
	var matches []catalog.Game
	for _, game := range games {
		if strings.Contains(game.ID, want) || strings.Contains(strings.ToLower(game.Title), want) {
			matches = append(matches, game)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return catalog.Game{}, severalGames(want, matches)
	}
	return catalog.Game{}, fmt.Errorf("No simulator or game called %s.\nDid you mean:\n%s", want, strings.Join(gameSuggestions(games, want), "\n"))
}
func severalGames(want string, games []catalog.Game) error {
	values := make([]string, 0, len(games))
	for _, game := range games {
		values = append(values, game.ID)
	}
	return fmt.Errorf("Several games match %s:\n%s", want, listMatches(values))
}
func gameSuggestions(games []catalog.Game, want string) []string {
	names := make([]string, len(games))
	titles := make([]string, len(games))
	for i, game := range games {
		names[i], titles[i] = game.ID, game.Title
	}
	return closest(want, names, titles, 5)
}

func (a *App) bold(value string) string {
	if a.NoColor || !a.IsTTY() {
		return value
	}
	return a.renderer().NewStyle().Bold(true).Render(value)
}
