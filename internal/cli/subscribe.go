package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) subscribeCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:     "subscribe",
		Short:   "Open the newsletter sign-up",
		Example: "  devops-daily subscribe --print",
		RunE: func(cmd *cobra.Command, args []string) error {
			url := "https://devops-daily.com/newsletters"
			if printOnly {
				fmt.Fprintln(a.Stdout, url)
			} else {
				fmt.Fprintln(a.Stdout, "Opening "+url)
				if err := a.OpenURL(url); err != nil {
					fmt.Fprintln(a.Stdout, "Could not open a browser. Open this link:")
					fmt.Fprintln(a.Stdout, url)
				}
			}
			fmt.Fprintln(a.Stdout, "The weekly DevOps Daily newsletter. Sign-up happens on the site; this tool never asks for your email.")
			return nil
		}}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the URL only")
	return cmd
}
