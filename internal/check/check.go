// Package check runs the network checks behind "devops-daily check": a TLS
// certificate, DNS records, HTTP security headers and a domain's email
// records. Each check talks only to the host being checked and the DNS
// server in use.
//
// The SSH app must never import this package: there a visitor could make our
// server connect to any host. cmd/ssh-site/imports_test.go fails if it does.
package check

import (
	"fmt"
	"strings"
	"time"
)

// DefaultTimeout is how long each check waits for one network reply.
const DefaultTimeout = 5 * time.Second

// Learn more pages on the site, one per check.
const (
	learnTLS     = "https://devops-daily.com/games/ssl-tls-handshake"
	learnDNS     = "https://devops-daily.com/games/dns-simulator"
	learnHeaders = "https://devops-daily.com/guides/owasp-top-10/05-security-misconfiguration"
	learnEmail   = "https://devops-daily.com/games/email-auth-lab"
)

// Status is the result of one finding.
type Status string

const (
	Pass    Status = "pass"
	Info    Status = "info"
	Warn    Status = "warn"
	Missing Status = "missing"
	Fail    Status = "fail"
)

// Severity is 0 for pass and info, 1 for warn and missing, 2 for fail. It is
// also the exit code a finding asks for.
func (s Status) Severity() int {
	switch s {
	case Fail:
		return 2
	case Warn, Missing:
		return 1
	}
	return 0
}

// Finding is one line of a report: what was looked at, what was found and,
// in plain words, what it means.
type Finding struct {
	ID      string `json:"id"`
	Status  Status `json:"status"`
	Label   string `json:"label"`
	Value   string `json:"value"`
	Meaning string `json:"meaning,omitempty"`
}

// Section groups findings under a heading, with an optional note that
// explains the whole group.
type Section struct {
	Title    string    `json:"title"`
	Note     string    `json:"note,omitempty"`
	Findings []Finding `json:"findings"`
}

func (s *Section) add(id string, status Status, label, value, meaning string) {
	s.Findings = append(s.Findings, Finding{ID: id, Status: status, Label: label, Value: value, Meaning: meaning})
}

// Report is the result of one check. Details holds the raw data for scripts.
type Report struct {
	Check     string    `json:"check"`
	Target    string    `json:"target"`
	Status    Status    `json:"status"`
	ExitCode  int       `json:"exitCode"`
	Warnings  int       `json:"warnings"`
	Failures  int       `json:"failures"`
	Sections  []Section `json:"sections"`
	Details   any       `json:"details"`
	LearnMore string    `json:"learnMore,omitempty"`
}

func (r *Report) add(s Section) {
	if len(s.Findings) > 0 {
		r.Sections = append(r.Sections, s)
	}
}

// finish counts warnings and failures and sets the status and exit code:
// 0 when all is well, 1 with warnings, 2 with at least one failure.
func (r *Report) finish() *Report {
	r.Warnings, r.Failures = 0, 0
	for _, s := range r.Sections {
		for _, f := range s.Findings {
			switch f.Status.Severity() {
			case 2:
				r.Failures++
			case 1:
				r.Warnings++
			}
		}
	}
	switch {
	case r.Failures > 0:
		r.Status, r.ExitCode = Fail, 2
	case r.Warnings > 0:
		r.Status, r.ExitCode = Warn, 1
	default:
		r.Status, r.ExitCode = Pass, 0
	}
	return r
}

func timeoutOr(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultTimeout
	}
	return d
}

// span formats a number of seconds as "45s", "5m", "1h30m" or "2d".
func span(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	units := []struct {
		name string
		size int64
	}{{"d", 86400}, {"h", 3600}, {"m", 60}, {"s", 1}}
	var parts []string
	for _, u := range units {
		if seconds >= u.size && len(parts) < 2 {
			parts = append(parts, fmt.Sprintf("%d%s", seconds/u.size, u.name))
			seconds %= u.size
		} else if len(parts) > 0 {
			break
		}
	}
	return strings.Join(parts, "")
}

func ttl(seconds uint32) string {
	if seconds < 60 {
		return fmt.Sprintf("TTL %ds", seconds)
	}
	return fmt.Sprintf("TTL %ds (%s)", seconds, span(int64(seconds)))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// list joins values, keeping at most max and saying how many were left out.
func list(values []string, max int) string {
	if len(values) <= max {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:max], ", ") + fmt.Sprintf(" and %d more", len(values)-max)
}
