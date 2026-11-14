package check

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func goodHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'nonce-abc' 'unsafe-inline'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("Permissions-Policy", "camera=(), geolocation=()")
	h.Set("Cache-Control", "public, max-age=300")
	h.Set("Age", "42")
	h.Set("ETag", `"v1"`)
	h.Set("Server", "nginx/1.27.0")
}

func TestHeadersFollowsRedirectsAndPasses(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/next", http.StatusMovedPermanently)
		case "/next":
			http.Redirect(w, r, "/final#frag", http.StatusFound)
		default:
			if r.Header.Get("User-Agent") != "test-agent" {
				t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
			}
			goodHeaders(w)
		}
	}))
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	r, err := Headers(context.Background(), srv.URL, HeadersOptions{RootCAs: pool, UserAgent: "test-agent"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 {
		t.Fatalf("exit %d: %+v", r.ExitCode, r.Sections)
	}
	d := r.Details.(*HeadersDetails)
	if len(d.Hops) != 3 || d.Hops[0].Status != 301 || d.Hops[1].Location != srv.URL+"/final" || d.Hops[2].Status != 200 || d.FinalURL != srv.URL+"/final" {
		t.Fatalf("hops = %+v, final %s", d.Hops, d.FinalURL)
	}
	for _, id := range []string{"https", "hsts", "csp", "nosniff", "framing", "referrer", "permissions"} {
		if f := finding(t, r, id); f.Status != Pass {
			t.Errorf("%s = %+v", id, f)
		}
	}
	if f := finding(t, r, "framing"); f.Value != "frame-ancestors 'none'" {
		t.Errorf("framing = %+v", f)
	}
	if f := finding(t, r, "cache-control"); f.Meaning != "Shared caches and CDNs may keep it; fresh for 5m." {
		t.Errorf("cache-control = %+v", f)
	}
	if f := finding(t, r, "server"); !strings.Contains(f.Meaning, "version number") {
		t.Errorf("server = %+v", f)
	}
	if d.Headers["ETag"] != `"v1"` || r.LearnMore != learnHeaders {
		t.Errorf("headers = %v", d.Headers)
	}
}

func TestHeadersMissingAndWeak(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer plain.Close()
	r, err := Headers(context.Background(), plain.URL, HeadersOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 1 {
		t.Fatalf("exit = %d", r.ExitCode)
	}
	want := map[string]Status{"https": Warn, "hsts": Info, "csp": Missing, "nosniff": Missing, "framing": Missing, "referrer": Missing, "permissions": Missing}
	for id, status := range want {
		if f := finding(t, r, id); f.Status != status {
			t.Errorf("%s = %+v, want %s", id, f, status)
		}
	}

	weak := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=300")
		h.Set("Content-Security-Policy", "default-src * 'unsafe-inline'")
		h.Set("X-Content-Type-Options", "yes")
		h.Set("X-Frame-Options", "ALLOW-FROM https://example.com")
		h.Set("Referrer-Policy", "no-referrer, unsafe-url")
		h.Set("Feature-Policy", "camera 'none'")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer weak.Close()
	pool := x509.NewCertPool()
	pool.AddCert(weak.Certificate())
	r, err = Headers(context.Background(), strings.TrimPrefix(weak.URL, "https://"), HeadersOptions{RootCAs: pool})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"status", "hsts", "csp", "nosniff", "framing", "referrer", "permissions"} {
		if f := finding(t, r, id); f.Status != Warn {
			t.Errorf("%s = %+v, want warn", id, f)
		}
	}
	if f := finding(t, r, "csp"); !strings.Contains(f.Meaning, "* 'unsafe-inline'") {
		t.Errorf("csp = %+v", f)
	}
	if f := finding(t, r, "referrer"); f.Value != "unsafe-url" {
		t.Errorf("referrer = %+v", f)
	}
}

func TestHeadersRedirectProblems(t *testing.T) {
	var plainURL string
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/a":
			http.Redirect(w, r, "/b", http.StatusFound)
		case r.URL.Path == "/b":
			http.Redirect(w, r, "/a", http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/n/"):
			var n int
			fmt.Sscanf(r.URL.Path, "/n/%d", &n)
			http.Redirect(w, r, fmt.Sprintf("/n/%d", n+1), http.StatusFound)
		}
	}))
	defer plain.Close()
	plainURL = plain.URL

	r, err := Headers(context.Background(), plainURL+"/a", HeadersOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "redirects"); f.Status != Fail || f.Label != "Loop" || r.ExitCode != 2 {
		t.Fatalf("loop = %+v", f)
	}
	r, err = Headers(context.Background(), plainURL+"/n/0", HeadersOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "redirects"); f.Label != "Too many" || len(r.Details.(*HeadersDetails).Hops) != MaxRedirects+1 {
		t.Fatalf("too many = %+v", f)
	}

	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plainURL+"/", http.StatusMovedPermanently)
	}))
	defer tlsSrv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(tlsSrv.Certificate())
	r, err = Headers(context.Background(), tlsSrv.URL, HeadersOptions{RootCAs: pool})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "downgrade"); f.Status != Warn {
		t.Fatalf("downgrade = %+v", f)
	}

	r, err = Headers(context.Background(), tlsSrv.URL, HeadersOptions{RootCAs: x509.NewCertPool(), Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "request"); f.Status != Fail || !strings.Contains(f.Meaning, "devops-daily check tls 127.0.0.1:") {
		t.Fatalf("untrusted = %+v", f)
	}

	for _, bad := range []string{"ftp://example.com", "https://", "exa mple.com"} {
		if _, err := Headers(context.Background(), bad, HeadersOptions{}); err == nil {
			t.Errorf("%q gave no error", bad)
		}
	}
}

func TestReferrerPolicyAndCSP(t *testing.T) {
	for in, want := range map[string]string{"": "", "no-referrer": "no-referrer", "unsafe-url, strict-origin": "strict-origin", "bogus": "bogus", "same-origin, bogus": "same-origin"} {
		if got := referrerPolicy(in); got != want {
			t.Errorf("referrerPolicy(%q) = %q, want %q", in, got, want)
		}
	}
	cases := map[string]Status{
		"default-src 'self'":                          Pass,
		"script-src 'self' 'unsafe-eval'":             Warn,
		"script-src 'unsafe-inline'":                  Warn,
		"script-src 'unsafe-inline' 'sha256-abc='":    Pass,
		"img-src 'self'":                              Warn,
		"script-src https:, default-src 'self'":       Pass,
		"default-src 'none'; script-src 'self' data:": Warn,
	}
	for policy, want := range cases {
		if got, _, _ := judgeCSP(parseCSP(policy)); got != want {
			t.Errorf("judgeCSP(%q) = %s, want %s", policy, got, want)
		}
	}
	if age, ok := maxAge(`max-age="600"; preload`); !ok || age != 600 {
		t.Errorf("maxAge = %d, %v", age, ok)
	}
}
