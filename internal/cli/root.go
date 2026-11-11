package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/browser"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/catalog"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/pager"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/paths"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
)

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type App struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	IsTTY      func() bool
	Width      func() int
	Now        func() time.Time
	Store      *catalog.Store
	OpenURL    func(string) error
	RunPager   func(string, string) error
	RunTUI     func(tea.Model) error
	HTTPClient *http.Client
	Build      BuildInfo
	Offline    bool
	NoColor    bool
	style      string
}

type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

func Main(args []string, info BuildInfo) int {
	app := DefaultApp(info)
	command := app.Command()
	command.SetArgs(args)
	if _, _, err := command.Find(args); err != nil {
		printError(app.Stderr, err)
		fmt.Fprintln(app.Stderr, "Run 'devops-daily --help' for usage.")
		return 2
	}
	if err := command.Execute(); err != nil {
		printError(app.Stderr, err)
		var use usageError
		if errors.As(err, &use) {
			fmt.Fprintln(app.Stderr, "Run 'devops-daily --help' for usage.")
			return 2
		}
		return 1
	}
	return 0
}

// printError writes an error for the terminal. Errors can carry text from
// the network (URLs, server replies), so control codes are stripped first.
func printError(w io.Writer, err error) {
	fmt.Fprintf(w, "devops-daily: %s\n", text.Clean(err.Error(), true))
}

func DefaultApp(info BuildInfo) *App {
	if info.Version == "dev" {
		if build, ok := debug.ReadBuildInfo(); ok && build.Main.Version != "" && build.Main.Version != "(devel)" {
			info.Version = build.Main.Version
		}
	}
	baseURL := os.Getenv("DEVOPS_DAILY_DATA_URL")
	userAgent := "devops-daily-cli/" + info.Version + " (+https://devops-daily.com/cli)"
	return &App{Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin, IsTTY: func() bool { return term.IsTerminal(os.Stdout.Fd()) }, Width: stdoutWidth, Now: time.Now, Store: catalog.New(paths.CacheDir(), baseURL, userAgent), OpenURL: browser.Open, RunPager: pager.Run, RunTUI: func(m tea.Model) error { _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); return err }, HTTPClient: http.DefaultClient, Build: info}
}

func stdoutWidth() int {
	if width, _, err := term.GetSize(os.Stdout.Fd()); err == nil && width > 0 {
		return width
	}
	if columns := os.Getenv("COLUMNS"); columns != "" {
		var width int
		if _, err := fmt.Sscanf(columns, "%d", &width); err == nil && width > 0 {
			return width
		}
	}
	return 80
}

func (a *App) init() {
	if a.Stdout == nil {
		a.Stdout = io.Discard
	}
	if a.Stderr == nil {
		a.Stderr = io.Discard
	}
	if a.Now == nil {
		a.Now = time.Now
	}
	if a.IsTTY == nil {
		a.IsTTY = func() bool { return false }
	}
	if a.Width == nil {
		a.Width = func() int { return 80 }
	}
	if a.Store == nil {
		a.Store = catalog.New(paths.CacheDir(), "", "devops-daily-cli/dev (+https://devops-daily.com/cli)")
	}
	if a.OpenURL == nil {
		a.OpenURL = browser.Open
	}
	if a.RunPager == nil {
		a.RunPager = pager.Run
	}
	if a.HTTPClient == nil {
		a.HTTPClient = http.DefaultClient
	}
}

func (a *App) Command() *cobra.Command {
	a.init()
	var offline, noColor bool
	root := &cobra.Command{
		Use:           "devops-daily",
		Short:         "Learn DevOps from your terminal",
		Example:       "  devops-daily                  the home menu\n  devops-daily posts --tag k8s\n  devops-daily read <slug>\n  devops-daily quiz docker\n  devops-daily play dns",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())}
			}
			return nil
		}, PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if raw := os.Getenv("DEVOPS_DAILY_DATA_URL"); raw != "" && !catalog.ValidBaseURL(raw) {
				return errors.New("DEVOPS_DAILY_DATA_URL must be an https:// URL (or http:// to localhost).")
			}
			a.Offline = offline || os.Getenv("DEVOPS_DAILY_OFFLINE") == "1"
			a.NoColor = noColor || os.Getenv("NO_COLOR") != ""
			return nil
		}, RunE: func(cmd *cobra.Command, args []string) error {
			if !a.IsTTY() {
				return cmd.Usage()
			}
			return a.home(cmd.Context())
		}}
	root.SetOut(a.Stdout)
	root.SetErr(a.Stderr)
	root.PersistentFlags().BoolVar(&offline, "offline", false, "use cached content only")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colour")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(a.postsCmd(), a.readCmd(), a.incidentCmd(), a.playCmd(), a.subscribeCmd(), a.syncCmd(), a.versionCmd(), a.updateCmd(), a.quizCmd(), a.flashcardsCmd(), a.checklistCmd())
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == root {
			fmt.Fprintln(a.Stdout, "Learn DevOps from your terminal: posts, quizzes, flashcards, checklists and")
			fmt.Fprintln(a.Stdout, "a daily incident from DevOps Daily. Run it with no command for the menu.")
			fmt.Fprintln(a.Stdout)
			cmd.Print(cmd.UsageString())
			fmt.Fprintln(a.Stdout, "Content: https://devops-daily.com\nNo tracking: the only network requests are the content download and \"update\".")
			return
		}
		fmt.Fprintln(a.Stdout, cmd.Short+".")
		fmt.Fprintln(a.Stdout)
		cmd.Print(cmd.UsageString())
	})
	return root
}

func (a *App) ensure(ctx context.Context) error {
	notice, err := a.Store.Ensure(ctx, a.Offline)
	if notice != "" {
		fmt.Fprintln(a.Stderr, a.muted(text.Clean(notice, false)))
	}
	return err
}

// colorStyle picks the Markdown style. Asking the terminal for its
// background can take a moment, so it happens once and only for the commands
// that render Markdown, before any full-screen program starts.
func (a *App) colorStyle() string {
	if a.style != "" {
		return a.style
	}
	switch {
	case a.NoColor || !a.IsTTY():
		a.style = "notty"
	case lipgloss.HasDarkBackground():
		a.style = "dark"
	default:
		a.style = "light"
	}
	return a.style
}
func (a *App) renderer() *lipgloss.Renderer {
	renderer := lipgloss.NewRenderer(a.Stdout)
	if a.NoColor || !a.IsTTY() {
		renderer.SetColorProfile(termenv.Ascii)
	}
	return renderer
}
func (a *App) muted(value string) string {
	if a.NoColor || !a.IsTTY() {
		return value
	}
	return a.renderer().NewStyle().Foreground(lipgloss.Color("#A1A1AA")).Render(value)
}
func (a *App) accent(value string) string {
	if a.NoColor || !a.IsTTY() {
		return value
	}
	return a.renderer().NewStyle().Foreground(lipgloss.Color("#F6A823")).Render(value)
}
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(n)(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
}
func maximumArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(n)(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
