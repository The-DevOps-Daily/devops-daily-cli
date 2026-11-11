package cli

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Show version",
		Example: "  devops-daily version",
		Run: func(cmd *cobra.Command, args []string) {
			version := a.Build.Version
			if version == "" {
				version = "dev"
			}
			parts := []string{}
			if a.Build.Commit != "" {
				parts = append(parts, "commit "+short(a.Build.Commit))
			}
			if a.Build.Date != "" {
				parts = append(parts, "built "+a.Build.Date)
			}
			parts = append(parts, runtime.Version()+" "+runtime.GOOS+"/"+runtime.GOARCH)
			fmt.Fprintln(a.Stdout, "devops-daily "+version+" ("+strings.Join(parts, ", ")+")")
		}}
}
func short(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
