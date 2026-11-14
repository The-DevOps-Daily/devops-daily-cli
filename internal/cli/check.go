package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/check"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/text"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

const checkLong = `Check a host from your own machine: its TLS certificate, DNS records, HTTP
security headers or email records. Only the host you name and the DNS server
are contacted. These checks are not part of the SSH app.

Exit codes: 0 all good, 1 warnings, 2 a problem or an error.`

func (a *App) checkCmd() *cobra.Command {
	var jsonOut bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:     "check",
		Short:   "Check a site's TLS, DNS, headers or email records",
		Long:    checkLong,
		Example: "  devops-daily check tls github.com\n  devops-daily check dns devops-daily.com --type MX\n  devops-daily check headers https://devops-daily.com\n  devops-daily check email gmail.com",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageError{fmt.Errorf("unknown check %q; use tls, dns, headers or email", args[0])}
			}
			return cmd.Help()
		},
	}
	cmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "print JSON")
	cmd.PersistentFlags().DurationVar(&timeout, "timeout", check.DefaultTimeout, "how long to wait for each network reply")
	run := func(cmd *cobra.Command, get func(context.Context) (*check.Report, error)) error {
		if timeout <= 0 {
			return usageError{fmt.Errorf("--timeout must be more than 0")}
		}
		report, err := get(cmd.Context())
		if err != nil {
			return usageError{err}
		}
		return a.printCheck(report, jsonOut)
	}

	tlsCmd := &cobra.Command{
		Use:     "tls <host[:port]>",
		Short:   "Check a TLS certificate: expiry, chain, trust and host name",
		Example: "  devops-daily check tls github.com\n  devops-daily check tls smtp.gmail.com:465",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func(ctx context.Context) (*check.Report, error) {
				return check.TLS(ctx, args[0], check.TLSOptions{Timeout: timeout, Now: a.Now})
			})
		},
	}

	var types []string
	var server string
	dnsCmd := &cobra.Command{
		Use:     "dns <name>",
		Short:   "Look up DNS records with their TTLs and the CNAME chain",
		Example: "  devops-daily check dns devops-daily.com\n  devops-daily check dns www.github.com --type CNAME,A --server 1.1.1.1",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func(ctx context.Context) (*check.Report, error) {
				return check.DNS(ctx, args[0], check.DNSOptions{Server: server, Types: types, Timeout: timeout})
			})
		},
	}
	dnsCmd.Flags().StringSliceVar(&types, "type", nil, "record types: A, AAAA, CNAME, MX, NS, TXT, CAA or SOA (default A,AAAA,MX,NS,TXT,CAA)")
	dnsCmd.Flags().StringVar(&server, "server", "", "DNS server IP to ask, like 1.1.1.1 (default: your system resolver)")

	headersCmd := &cobra.Command{
		Use:     "headers <url>",
		Short:   "Follow redirects and check security and cache headers",
		Example: "  devops-daily check headers devops-daily.com\n  devops-daily check headers http://example.com/login",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func(ctx context.Context) (*check.Report, error) {
				return check.Headers(ctx, args[0], check.HeadersOptions{Timeout: timeout, UserAgent: "devops-daily-cli/" + a.version() + " (check headers)"})
			})
		},
	}

	var selectors []string
	var emailServer string
	emailCmd := &cobra.Command{
		Use:     "email <domain>",
		Short:   "Check MX, SPF, DMARC, DKIM, MTA-STS and TLS-RPT records",
		Example: "  devops-daily check email gmail.com\n  devops-daily check email example.com --selector mailjet",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, func(ctx context.Context) (*check.Report, error) {
				return check.Email(ctx, args[0], check.EmailOptions{Server: emailServer, Selectors: selectors, Timeout: timeout})
			})
		},
	}
	emailCmd.Flags().StringSliceVar(&selectors, "selector", nil, "extra DKIM selector to try (repeat or comma-separate)")
	emailCmd.Flags().StringVar(&emailServer, "server", "", "DNS server IP to ask, like 1.1.1.1 (default: your system resolver)")

	for _, sub := range []*cobra.Command{tlsCmd, dnsCmd, headersCmd, emailCmd} {
		sub.Long = sub.Short + ".\n\n" + checkLong
	}
	cmd.AddCommand(tlsCmd, dnsCmd, headersCmd, emailCmd)
	return cmd
}

func (a *App) version() string {
	if a.Build.Version == "" {
		return "dev"
	}
	return a.Build.Version
}

// printCheck prints the report and turns its result into the exit code.
func (a *App) printCheck(report *check.Report, jsonOut bool) error {
	if jsonOut {
		if err := json.NewEncoder(a.Stdout).Encode(report); err != nil {
			return exitError{code: 2, err: err}
		}
	} else {
		a.printReport(report)
	}
	if report.ExitCode != 0 {
		return exitError{code: report.ExitCode}
	}
	return nil
}

var checkTitles = map[string]string{"tls": "TLS check", "dns": "DNS check", "headers": "Header check", "email": "Email check"}

const statusWidth = 9 // "missing" and two spaces

func (a *App) printReport(r *check.Report) {
	width := max(a.width(), 40)
	out := a.Stdout
	title := checkTitles[r.Check] + " for " + text.Line(r.Target, 200)
	if r.Unicode != "" {
		title += " (" + text.Line(r.Unicode, 200) + ")"
	}
	fmt.Fprintln(out, a.bold(title))
	for _, s := range r.Sections {
		fmt.Fprintln(out)
		fmt.Fprintln(out, a.bold(text.Line(s.Title, 80)))
		if s.Note != "" {
			for _, line := range wrapHard(text.Line(s.Note, 0), width-2) {
				fmt.Fprintln(out, "  "+a.muted(line))
			}
		}
		statusCol, labelWidth := false, 0
		for _, f := range s.Findings {
			statusCol = statusCol || f.Status != check.Info
			labelWidth = max(labelWidth, utf8.RuneCountInString(f.Label))
		}
		labelWidth = min(labelWidth, 26)
		for _, f := range s.Findings {
			prefix, plain := "  ", 2
			if statusCol {
				prefix += a.status(f.Status, tui.PadRight(string(f.Status), statusWidth))
				plain += statusWidth
			}
			if labelWidth > 0 {
				prefix += tui.PadRight(text.Line(f.Label, labelWidth), labelWidth) + "  "
				plain += labelWidth + 2
			}
			value := text.Line(f.Value, 0)
			if width-plain < 24 {
				fmt.Fprintln(out, strings.TrimRight(prefix, " "))
				prefix, plain = "      ", 6
			}
			lines := wrapHard(value, width-plain)
			if len(lines) == 0 {
				lines = []string{""}
			}
			fmt.Fprintln(out, strings.TrimRight(prefix+lines[0], " "))
			for _, line := range lines[1:] {
				fmt.Fprintln(out, strings.Repeat(" ", plain)+line)
			}
			if f.Meaning != "" {
				// Under the label when there is a status column, else under the value.
				indent := 2 + statusWidth
				if !statusCol {
					indent = plain
				}
				for _, line := range wrapHard(text.Line(f.Meaning, 0), width-indent) {
					fmt.Fprintln(out, strings.Repeat(" ", indent)+a.muted(line))
				}
			}
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, a.status(r.Status, summary(r)))
	if r.LearnMore != "" {
		fmt.Fprintln(out, "Learn more: "+r.LearnMore)
	}
}

func summary(r *check.Report) string {
	warnings := fmt.Sprintf("%d warnings", r.Warnings)
	if r.Warnings == 1 {
		warnings = "1 warning"
	}
	switch {
	case r.Failures > 0:
		problems := fmt.Sprintf("%d problems", r.Failures)
		if r.Failures == 1 {
			problems = "1 problem"
		}
		if r.Warnings > 0 {
			return "Result: " + problems + " and " + warnings + "."
		}
		return "Result: " + problems + "."
	case r.Warnings > 0:
		return "Result: " + warnings + ", no problems."
	}
	return "Result: all good."
}

func (a *App) status(s check.Status, value string) string {
	if a.NoColor || !a.IsTTY() {
		return value
	}
	style := a.renderer().NewStyle()
	switch s {
	case check.Pass:
		style = style.Foreground(lipgloss.Color("#22C55E"))
	case check.Warn, check.Missing:
		style = style.Foreground(lipgloss.Color("#F6A823"))
	case check.Fail:
		style = style.Foreground(lipgloss.Color("#EF4444")).Bold(true)
	default:
		style = style.Foreground(lipgloss.Color("#A1A1AA"))
	}
	return style.Render(value)
}

// wrapHard wraps value at spaces and also breaks words longer than width,
// such as long TXT records.
func wrapHard(value string, width int) []string {
	width = max(width, 10)
	var lines []string
	line := ""
	for _, word := range strings.Fields(value) {
		for utf8.RuneCountInString(word) > width {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			runes := []rune(word)
			lines = append(lines, string(runes[:width]))
			word = string(runes[width:])
		}
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width:
			lines = append(lines, line)
			line = word
		default:
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
