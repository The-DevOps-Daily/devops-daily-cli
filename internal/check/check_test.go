package check

import (
	"testing"
)

func finding(t *testing.T, r *Report, id string) Finding {
	t.Helper()
	for _, s := range r.Sections {
		for _, f := range s.Findings {
			if f.ID == id {
				return f
			}
		}
	}
	t.Fatalf("no finding %q in report: %+v", id, r.Sections)
	return Finding{}
}

func hasFinding(r *Report, id string) bool {
	for _, s := range r.Sections {
		for _, f := range s.Findings {
			if f.ID == id {
				return true
			}
		}
	}
	return false
}

func TestFinishSetsExitCode(t *testing.T) {
	cases := []struct {
		statuses []Status
		code     int
		status   Status
	}{
		{[]Status{Pass, Info}, 0, Pass},
		{[]Status{Pass, Missing}, 1, Warn},
		{[]Status{Warn, Info}, 1, Warn},
		{[]Status{Warn, Fail, Pass}, 2, Fail},
		{nil, 0, Pass},
	}
	for _, c := range cases {
		s := Section{Title: "x"}
		for i, status := range c.statuses {
			s.add(string(rune('a'+i)), status, "", "", "")
		}
		r := &Report{}
		r.add(s)
		r.finish()
		if r.ExitCode != c.code || r.Status != c.status {
			t.Errorf("%v: exit %d status %s, want %d %s", c.statuses, r.ExitCode, r.Status, c.code, c.status)
		}
	}
}

func TestHostPort(t *testing.T) {
	cases := []struct{ in, host, port string }{
		{"github.com", "github.com", "443"},
		{"GitHub.com.", "github.com", "443"},
		{"example.com:8443", "example.com", "8443"},
		{"https://example.com/path?q=1", "example.com", "443"},
		{"https://example.com:444/", "example.com", "444"},
		{"127.0.0.1:9000", "127.0.0.1", "9000"},
		{"[::1]:9000", "::1", "9000"},
		{"[::1]", "::1", "443"},
		{"::1", "::1", "443"},
	}
	for _, c := range cases {
		host, port, err := hostPort(c.in, "443")
		if err != nil || host != c.host || port != c.port {
			t.Errorf("hostPort(%q) = %q, %q, %v; want %q, %q", c.in, host, port, err, c.host, c.port)
		}
	}
	for _, bad := range []string{"", "exa mple.com", "example.com:0", "example.com:99999", "example.com:http", "-bad-.com:x"} {
		if _, _, err := hostPort(bad, "443"); err == nil {
			t.Errorf("hostPort(%q) gave no error", bad)
		}
	}
}

func TestDomainName(t *testing.T) {
	for in, want := range map[string]string{"Example.COM.": "example.com", "me@example.com": "example.com", "https://example.com/x": "example.com", "_dmarc.example.com": "_dmarc.example.com"} {
		if got, err := domainName(in); err != nil || got != want {
			t.Errorf("domainName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "1.1.1.1", "a..b", "bad name.com"} {
		if _, err := domainName(bad); err == nil {
			t.Errorf("domainName(%q) gave no error", bad)
		}
	}
}

func TestSpanAndTTL(t *testing.T) {
	for seconds, want := range map[int64]string{0: "0s", 45: "45s", 60: "1m", 90: "1m30s", 3600: "1h", 5400: "1h30m", 86400: "1d", 31536000: "365d"} {
		if got := span(seconds); got != want {
			t.Errorf("span(%d) = %q, want %q", seconds, got, want)
		}
	}
	if got := ttl(300); got != "TTL 300s (5m)" {
		t.Errorf("ttl(300) = %q", got)
	}
	if got := ttl(30); got != "TTL 30s" {
		t.Errorf("ttl(30) = %q", got)
	}
}

func TestList(t *testing.T) {
	if got := list([]string{"a", "b", "c"}, 2); got != "a, b and 1 more" {
		t.Errorf("list = %q", got)
	}
	if got := list([]string{"a", "b"}, 2); got != "a, b" {
		t.Errorf("list = %q", got)
	}
}
