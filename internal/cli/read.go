package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/render"
)

func (a *App) readCmd() *cobra.Command {
	var noPager bool
	var style string
	cmd := &cobra.Command{
		Use:     "read <slug>",
		Short:   "Read a post",
		Example: "  devops-daily read kubernetes-basics",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.ensure(cmd.Context()); err != nil {
				return err
			}
			posts, err := a.Store.Posts()
			if err != nil {
				return err
			}
			post, err := findPost(posts, args[0])
			if err != nil {
				return err
			}
			body, err := a.Store.PostBody(post)
			if err != nil {
				return err
			}
			if !a.IsTTY() || a.NoColor {
				style = "notty"
			} else if style == "auto" {
				style = a.colorStyle()
			}
			output, err := render.Article(post, body, a.width(), style)
			if err != nil {
				return err
			}
			if a.IsTTY() && !noPager {
				return a.RunPager(post.Title, output)
			}
			_, err = fmt.Fprintln(a.Stdout, output)
			return err
		}}
	cmd.Flags().BoolVar(&noPager, "no-pager", false, "print instead of paging")
	cmd.Flags().StringVar(&style, "style", "auto", "auto, dark, light or notty")
	return cmd
}
