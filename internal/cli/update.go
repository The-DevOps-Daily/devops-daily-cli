package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

func UpgradeMethod(executable string) string {
	if strings.Contains(executable, "/Cellar/") || strings.Contains(executable, "/homebrew/") || strings.Contains(executable, "/linuxbrew/") {
		return "brew upgrade devops-daily"
	}
	if strings.Contains(executable, "node_modules") {
		return "npm install -g @devopsdaily/devops-daily-cli@latest"
	}
	bin := os.Getenv("GOBIN")
	if bin == "" {
		root := os.Getenv("GOPATH")
		if root == "" {
			if home, err := os.UserHomeDir(); err == nil {
				root = filepath.Join(home, "go")
			}
		}
		bin = filepath.Join(root, "bin")
	}
	if bin != "" && strings.HasPrefix(filepath.Clean(executable), filepath.Clean(bin)+string(os.PathSeparator)) {
		return "go install github.com/The-DevOps-Daily/devops-daily-cli/cmd/devops-daily@latest"
	}
	return "curl -fsSL https://raw.githubusercontent.com/The-DevOps-Daily/devops-daily-cli/main/install.sh | sh"
}

func (a *App) updateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "update",
		Short:   "Check for a newer version",
		Example: "  devops-daily update",
		RunE: func(cmd *cobra.Command, args []string) error {
			current := a.Build.Version
			if current == "" {
				current = "dev"
			}
			fmt.Fprintln(a.Stdout, "Current version: "+current)
			if current == "dev" {
				fmt.Fprintln(a.Stdout, "This is a development build.")
			} else {
				a.checkUpdate(cmd.Context(), current)
			}
			executable, _ := os.Executable()
			fmt.Fprintln(a.Stdout, "To upgrade: "+UpgradeMethod(executable))
			return nil
		}}
}
func (a *App) checkUpdate(parent context.Context, current string) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/The-DevOps-Daily/devops-daily-cli/releases/latest", nil)
	if err != nil {
		a.updateError(err)
		return
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := a.HTTPClient.Do(request)
	if err != nil {
		a.updateError(err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		fmt.Fprintln(a.Stdout, "No release is published yet.")
		return
	}
	if response.StatusCode != http.StatusOK {
		a.updateError(fmt.Errorf("GitHub returned %s", response.Status))
		return
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		a.updateError(err)
		return
	}
	if strings.TrimPrefix(release.TagName, "v") == strings.TrimPrefix(current, "v") {
		fmt.Fprintln(a.Stdout, "You have the latest version.")
	} else {
		fmt.Fprintf(a.Stdout, "Version %s is out (you have %s).\n", release.TagName, current)
	}
}
func (a *App) updateError(err error) {
	fmt.Fprintf(a.Stdout, "Could not check the latest version (%s).\n", text.Line(err.Error(), 120))
}
