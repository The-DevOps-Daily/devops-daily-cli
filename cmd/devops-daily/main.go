// Command devops-daily is the local DevOps Daily terminal app.
package main

import (
	"os"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/cli"
)

var version, commit, date = "dev", "", ""

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
