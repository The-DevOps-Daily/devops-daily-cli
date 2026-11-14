package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/check"
)

func TestCheckHeadersTextAndJSON(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}))
	defer site.Close()

	if code := run(t, app, "check", "headers", site.URL); code != 1 {
		t.Fatalf("exit code = %d, stderr = %s", code, calls.stderr.String())
	}
	out := calls.stdout.String()
	for _, want := range []string{"Header check for " + site.URL + "/", "Requests", "Security headers", "pass     X-Content-Type-Options", "missing  Content-Security-Policy", "Result: ", "warnings, no problems.", "Learn more: https://devops-daily.com/guides/owasp-top-10/05-security-misconfiguration"} {
		if !strings.Contains(out, want) {
			t.Errorf("output has no %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b") || strings.Contains(strings.ToLower(out), "sponsor") || strings.Contains(out, "Acme") {
		t.Errorf("output has escape codes or a sponsor line:\n%s", out)
	}

	if code := run(t, app, "check", "headers", site.URL, "--json"); code != 1 {
		t.Fatalf("JSON exit code = %d", code)
	}
	var report struct {
		Check    string `json:"check"`
		ExitCode int    `json:"exitCode"`
		Details  struct {
			Hops []struct {
				Status int `json:"status"`
			} `json:"hops"`
		} `json:"details"`
	}
	if err := json.Unmarshal(calls.stdout.Bytes(), &report); err != nil {
		t.Fatalf("JSON: %v\n%s", err, calls.stdout.String())
	}
	if report.Check != "headers" || report.ExitCode != 1 || len(report.Details.Hops) != 1 || report.Details.Hops[0].Status != 200 {
		t.Fatalf("report = %+v", report)
	}

	if got := srv.count("index.json"); got != 0 {
		t.Fatalf("check downloaded content: %d index requests", got)
	}
}

func TestCheckTLSUntrustedExitsTwo(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer site.Close()
	if code := run(t, app, "check", "tls", site.Listener.Addr().String(), "--timeout", "2s"); code != 2 {
		t.Fatalf("exit code = %d, stdout = %s, stderr = %s", code, calls.stdout.String(), calls.stderr.String())
	}
	out := calls.stdout.String()
	for _, want := range []string{"TLS check for 127.0.0.1:", "fail     Trusted", "Expires in", "Chain", "Result: ", "Learn more: https://devops-daily.com/games/ssl-tls-handshake"} {
		if !strings.Contains(out, want) {
			t.Errorf("output has no %q:\n%s", want, out)
		}
	}
}

func TestCheckUsageErrors(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	cases := [][]string{
		{"check", "nope"},
		{"check", "tls"},
		{"check", "tls", "a", "b"},
		{"check", "tls", "bad host"},
		{"check", "dns", "example.com", "--type", "PTR"},
		{"check", "dns", "example.com", "--server", "dns.google"},
		{"check", "email", "1.1.1.1"},
		{"check", "headers", "ftp://example.com"},
		{"check", "headers", "example.com", "--timeout", "0s"},
		{"check", "headers", "example.com", "--timeout", "soon"},
	}
	for _, args := range cases {
		if code := run(t, app, args...); code != 2 {
			t.Errorf("%v exit code = %d, stderr = %s", args, code, calls.stderr.String())
		}
	}
	if code := run(t, app, "check"); code != 0 {
		t.Fatalf("check help exit code = %d", code)
	}
	out := calls.stdout.String()
	for _, want := range []string{"Exit codes: 0 all good, 1 warnings, 2 a problem or an error.", "not part of the SSH app", "tls", "dns", "headers", "email"} {
		if !strings.Contains(out, want) {
			t.Errorf("check help has no %q:\n%s", want, out)
		}
	}
	if code := run(t, app, "--help"); code != 0 || !strings.Contains(calls.stdout.String(), "check") {
		t.Fatalf("root help = %s", calls.stdout.String())
	}
}

func TestPrintReportCleansAndWraps(t *testing.T) {
	srv := newTestServer(t)
	app, calls := newTestApp(t, srv)
	app.Width = func() int { return 50 }
	report := &check.Report{Check: "dns", Target: "evil\x1b]2;owned\x07.test", Status: check.Warn, Warnings: 1, ExitCode: 1, Sections: []check.Section{{
		Title: "TXT records",
		Findings: []check.Finding{
			{ID: "txt", Status: check.Warn, Label: "TTL 300s (5m)", Value: "v=spf1 \x1b[2Jinclude:" + strings.Repeat("x", 80), Meaning: "A long meaning that has to wrap onto more than one line at this width."},
		},
	}}}
	if err := app.printCheck(report, false); err == nil {
		t.Fatal("a report with warnings returned no exit error")
	}
	out := calls.stdout.String()
	if strings.ContainsAny(out, "\x1b\x07") || strings.Contains(out, "owned") || strings.Contains(out, "[2J") {
		t.Fatalf("output has escape sequences: %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 50 {
			t.Errorf("line longer than 50: %q", line)
		}
	}
	if !strings.Contains(out, "Result: 1 warning, no problems.") {
		t.Errorf("summary missing:\n%s", out)
	}
	if err := app.printCheck(&check.Report{Check: "dns", Status: check.Pass}, false); err != nil {
		t.Fatalf("a clean report returned %v", err)
	}
}
