package check

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxRedirects is how many redirects "check headers" follows.
const MaxRedirects = 10

type HeadersOptions struct {
	Timeout   time.Duration
	RootCAs   *x509.CertPool // nil: the system's trusted roots
	UserAgent string
}

type Hop struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Location string `json:"location,omitempty"`
}

type HeadersDetails struct {
	URL      string            `json:"url"`
	FinalURL string            `json:"finalUrl,omitempty"`
	Hops     []Hop             `json:"hops"`
	Headers  map[string]string `json:"headers"`
	Error    string            `json:"error,omitempty"`
}

// reportedHeaders are copied to the JSON details as the server sent them.
var reportedHeaders = []string{
	"Strict-Transport-Security", "Content-Security-Policy", "Content-Security-Policy-Report-Only",
	"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Permissions-Policy",
	"Cache-Control", "Age", "ETag", "Server",
}

// Headers requests rawURL (https:// is added when there is no scheme),
// follows redirects one at a time and checks the final response's headers.
// The error is only for a URL that cannot be used.
func Headers(ctx context.Context, rawURL string, opts HeadersOptions) (*Report, error) {
	target, err := parseURL(rawURL)
	if err != nil {
		return nil, err
	}
	timeout := timeoutOr(opts.Timeout)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: opts.RootCAs}
	client := &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()

	d := &HeadersDetails{URL: target.String(), Hops: []Hop{}, Headers: map[string]string{}}
	r := &Report{Check: "headers", Target: target.String(), Details: d, LearnMore: learnHeaders}
	hops := Section{Title: "Requests", Note: "Each request, its HTTP status and where a redirect sent you next."}
	seen := map[string]bool{}
	current := target
	var final *http.Response
	for i := 0; ; i++ {
		if i > MaxRedirects {
			hops.add("redirects", Fail, "Too many", fmt.Sprintf("more than %d redirects", MaxRedirects), "Browsers give up too. Look for a redirect loop between two addresses.")
			break
		}
		if seen[current.String()] {
			hops.add("redirects", Fail, "Loop", "redirects back to "+current.String(), "The redirects go round in a circle, so the page never loads.")
			break
		}
		seen[current.String()] = true
		resp, err := get(ctx, client, current, opts.UserAgent)
		if err != nil {
			d.Error = err.Error()
			hops.add("request", Fail, fmt.Sprintf("%d.", i+1), current.String(), requestProblem(err, current, timeout))
			break
		}
		resp.Body.Close()
		hop := Hop{URL: current.String(), Status: resp.StatusCode}
		location := resp.Header.Get("Location")
		if resp.StatusCode < 300 || resp.StatusCode > 399 || location == "" {
			d.Hops = append(d.Hops, hop)
			hops.add("hop", Info, fmt.Sprintf("%d.", i+1), fmt.Sprintf("%d %s", resp.StatusCode, current), "")
			final = resp
			break
		}
		next, err := current.Parse(location)
		if err != nil || (next.Scheme != "http" && next.Scheme != "https") {
			d.Hops = append(d.Hops, hop)
			hops.add("hop", Fail, fmt.Sprintf("%d.", i+1), fmt.Sprintf("%d %s -> %s", resp.StatusCode, current, location), "The redirect target is not a usable http or https URL.")
			break
		}
		next.Fragment = ""
		hop.Location = next.String()
		d.Hops = append(d.Hops, hop)
		hops.add("hop", Info, fmt.Sprintf("%d.", i+1), fmt.Sprintf("%d %s -> %s", resp.StatusCode, current, next), "")
		if current.Scheme == "https" && next.Scheme == "http" {
			hops.add("downgrade", Warn, "Downgrade", "redirects from HTTPS to plain HTTP", "Anyone on the network can read and change the plain HTTP request.")
		}
		current = next
	}
	r.add(hops)
	if final == nil {
		return r.finish(), nil
	}
	d.FinalURL = current.String()
	for _, h := range reportedHeaders {
		if values := final.Header.Values(h); len(values) > 0 {
			d.Headers[h] = strings.Join(values, ", ")
		}
	}

	page := Section{Title: "Final response"}
	if current.Scheme == "https" {
		page.add("https", Pass, "HTTPS", "yes", "The page is encrypted in transit.")
	} else {
		page.add("https", Warn, "HTTPS", "no, plain HTTP", "Anyone on the network can read and change the page. Serve it over HTTPS and redirect HTTP to it.")
	}
	if final.StatusCode >= 400 {
		page.add("status", Warn, "Status", fmt.Sprint(final.StatusCode), "An error page. Your real pages may send different headers; try one that loads.")
	} else {
		page.add("status", Info, "Status", fmt.Sprint(final.StatusCode), "")
	}
	r.add(page)
	r.add(securityHeaders(final.Header, current.Scheme == "https"))
	r.add(cacheHeaders(final.Header))
	return r.finish(), nil
}

func parseURL(raw string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not a URL, like https://example.com", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%q: only http:// and https:// URLs can be checked", raw)
	}
	if _, _, err := hostPort(u.Host, "443"); err != nil {
		return nil, err
	}
	u.Fragment, u.User = "", nil
	if u.Path == "" {
		u.Path = "/"
	}
	return u, nil
}

func get(ctx context.Context, client *http.Client, u *url.URL, userAgent string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	return client.Do(req)
}

func requestProblem(err error, u *url.URL, timeout time.Duration) string {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var hostname x509.HostnameError
	switch {
	case errors.As(err, &unknown), errors.As(err, &invalid), errors.As(err, &hostname):
		return "The certificate is not valid, so browsers would stop here. Run: devops-daily check tls " + u.Host
	case isTimeout(err):
		return fmt.Sprintf("No answer within %s.", timeout)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return "The request failed: " + err.Error()
}

func securityHeaders(h http.Header, https bool) Section {
	s := Section{Title: "Security headers", Note: "Headers that turn on browser protections. Each one closes a common attack."}
	csp := strings.Join(h.Values("Content-Security-Policy"), ", ")
	policies := parseCSP(csp)

	hsts := h.Get("Strict-Transport-Security")
	switch {
	case !https:
		s.add("hsts", Info, "Strict-Transport-Security", "not checked: the page is plain HTTP", "HSTS only works over HTTPS. Move the site to HTTPS first.")
	case hsts == "":
		s.add("hsts", Missing, "Strict-Transport-Security", "missing", "Tells browsers to always use HTTPS here, so a visit cannot be downgraded to plain HTTP.")
	default:
		age, ok := maxAge(hsts)
		switch {
		case !ok:
			s.add("hsts", Warn, "Strict-Transport-Security", hsts, "There is no valid max-age, so browsers ignore it. Use max-age=31536000 (one year).")
		case age == 0:
			s.add("hsts", Warn, "Strict-Transport-Security", hsts, "max-age=0 switches HSTS off.")
		case age < 15552000:
			s.add("hsts", Warn, "Strict-Transport-Security", hsts, fmt.Sprintf("Browsers remember HTTPS-only for %s; 180 days or more is the usual advice (one year is common).", span(age)))
		default:
			s.add("hsts", Pass, "Strict-Transport-Security", hsts, fmt.Sprintf("Browsers will use only HTTPS here for %s after a visit.", plural(int(age/86400), "day", "days")))
		}
	}

	switch {
	case csp != "":
		status, reason, scripts := judgeCSP(policies)
		value := csp
		if len(csp) > 200 && scripts != "" {
			others := -1
			for _, p := range policies {
				others += len(p)
			}
			value = scripts + " (and " + plural(others, "more directive", "more directives") + "; --json shows the whole policy)"
		}
		s.add("csp", status, "Content-Security-Policy", value, reason)
	case h.Get("Content-Security-Policy-Report-Only") != "":
		s.add("csp", Warn, "Content-Security-Policy", "report-only", "Content-Security-Policy-Report-Only reports problems but blocks nothing. Switch to Content-Security-Policy when the reports are clean.")
	default:
		s.add("csp", Missing, "Content-Security-Policy", "missing", "Limits where scripts and other content may load from, which stops most cross-site scripting.")
	}

	nosniff := strings.TrimSpace(h.Get("X-Content-Type-Options"))
	switch {
	case strings.EqualFold(nosniff, "nosniff"):
		s.add("nosniff", Pass, "X-Content-Type-Options", nosniff, "Browsers will not guess a file's type, so an uploaded text file cannot run as a script.")
	case nosniff == "":
		s.add("nosniff", Missing, "X-Content-Type-Options", "missing", "Set it to nosniff, so browsers do not guess file types and run, say, an uploaded text file as a script.")
	default:
		s.add("nosniff", Warn, "X-Content-Type-Options", nosniff, "The only valid value is nosniff.")
	}

	frame := strings.TrimSpace(h.Get("X-Frame-Options"))
	ancestors, hasAncestors := directive(policies, "frame-ancestors")
	switch {
	case hasAncestors && (ancestors == "*" || ancestors == "https:" || ancestors == "http:"):
		s.add("framing", Warn, "Frame protection", "frame-ancestors "+ancestors, "Any site may show this page in a frame, which allows clickjacking.")
	case hasAncestors:
		s.add("framing", Pass, "Frame protection", "frame-ancestors "+ancestors, "The CSP decides which sites may show this page in a frame (stops clickjacking).")
	case strings.EqualFold(frame, "DENY") || strings.EqualFold(frame, "SAMEORIGIN"):
		s.add("framing", Pass, "Frame protection", "X-Frame-Options: "+frame, "Other sites cannot show this page in a frame (stops clickjacking).")
	case frame != "":
		s.add("framing", Warn, "Frame protection", "X-Frame-Options: "+frame, "Browsers only know DENY and SAMEORIGIN (ALLOW-FROM is ignored). Use the CSP frame-ancestors directive instead.")
	default:
		s.add("framing", Missing, "Frame protection", "missing (no X-Frame-Options or CSP frame-ancestors)", "Without it, other sites can show this page in a hidden frame and trick clicks (clickjacking).")
	}

	referrer := referrerPolicy(h.Get("Referrer-Policy"))
	switch referrer {
	case "":
		s.add("referrer", Missing, "Referrer-Policy", "missing", "Browsers then use strict-origin-when-cross-origin, a safe default. Set it to make the choice explicit.")
	case "unsafe-url", "no-referrer-when-downgrade":
		s.add("referrer", Warn, "Referrer-Policy", referrer, "Sends the full URL, with paths and query strings, to other sites. Use strict-origin-when-cross-origin.")
	case "no-referrer", "same-origin", "strict-origin", "strict-origin-when-cross-origin", "origin", "origin-when-cross-origin":
		s.add("referrer", Pass, "Referrer-Policy", referrer, "Controls how much of the page URL other sites see when a visitor follows a link.")
	default:
		s.add("referrer", Warn, "Referrer-Policy", referrer, "Not a value browsers know, so they ignore it.")
	}

	permissions := h.Get("Permissions-Policy")
	switch {
	case permissions != "":
		s.add("permissions", Pass, "Permissions-Policy", permissions, "Turns off browser features (camera, microphone, location and so on) the page does not use.")
	case h.Get("Feature-Policy") != "":
		s.add("permissions", Warn, "Permissions-Policy", "only the old Feature-Policy", "Feature-Policy is the old name. Browsers now read Permissions-Policy.")
	default:
		s.add("permissions", Missing, "Permissions-Policy", "missing", "Lets you turn off browser features (camera, microphone, location) the page does not use.")
	}
	return s
}

var versionPattern = regexp.MustCompile(`/v?\d+(\.\d+)*`)

func cacheHeaders(h http.Header) Section {
	s := Section{Title: "Caching and server"}
	if cc := strings.Join(h.Values("Cache-Control"), ", "); cc != "" {
		s.add("cache-control", Info, "Cache-Control", cc, cacheMeaning(cc))
	} else {
		s.add("cache-control", Info, "Cache-Control", "not set", "Browsers and CDNs guess how long to keep the page from Last-Modified.")
	}
	if age := h.Get("Age"); age != "" {
		meaning := "How long this copy has been in a cache: a CDN or proxy served it."
		if seconds, err := strconv.ParseInt(age, 10, 64); err == nil {
			meaning = fmt.Sprintf("This copy has been in a cache (a CDN or proxy) for %s.", span(seconds))
		}
		s.add("age", Info, "Age", age, meaning)
	} else {
		s.add("age", Info, "Age", "not set", "No cache in front of the server says it served this copy, so it probably came from the server itself.")
	}
	if etag := h.Get("ETag"); etag != "" {
		s.add("etag", Info, "ETag", etag, "A version tag. Browsers send it back to ask \"has this changed?\" and get a short 304 reply if not.")
	} else {
		s.add("etag", Info, "ETag", "not set", "Without it (or Last-Modified), browsers download the page again instead of revalidating it.")
	}
	if server := h.Get("Server"); server != "" {
		meaning := "The software that answered, as the server reports it."
		if versionPattern.MatchString(server) {
			meaning = "It shows a version number, which helps attackers match known bugs. Most servers can hide it (nginx: server_tokens off)."
		}
		s.add("server", Info, "Server", server, meaning)
	} else {
		s.add("server", Info, "Server", "not set", "")
	}
	return s
}

func cacheMeaning(cc string) string {
	var parts []string
	for _, item := range strings.Split(strings.ToLower(cc), ",") {
		key, value, _ := strings.Cut(strings.TrimSpace(item), "=")
		seconds, err := strconv.ParseInt(strings.Trim(value, `"`), 10, 64)
		switch {
		case key == "no-store":
			parts = append(parts, "nothing may store it")
		case key == "no-cache":
			parts = append(parts, "caches must check with the server before each reuse")
		case key == "private":
			parts = append(parts, "only the browser may keep it, not shared caches or CDNs")
		case key == "public":
			parts = append(parts, "shared caches and CDNs may keep it")
		case key == "max-age" && err == nil && seconds == 0:
			parts = append(parts, "stale at once")
		case key == "max-age" && err == nil:
			parts = append(parts, "fresh for "+span(seconds))
		case key == "s-maxage" && err == nil:
			parts = append(parts, "CDNs keep it for "+span(seconds))
		case key == "must-revalidate":
			parts = append(parts, "once stale, it must be checked before reuse")
		case key == "immutable":
			parts = append(parts, "it never changes while fresh")
		}
	}
	if len(parts) == 0 {
		return "Tells browsers and CDNs how long they may keep a copy."
	}
	text := strings.Join(parts, "; ")
	return strings.ToUpper(text[:1]) + text[1:] + "."
}

func maxAge(hsts string) (int64, bool) {
	for _, part := range strings.Split(hsts, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(strings.TrimSpace(key), "max-age") {
			n, err := strconv.ParseInt(strings.Trim(strings.TrimSpace(value), `"`), 10, 64)
			return n, err == nil && n >= 0
		}
	}
	return 0, false
}

// parseCSP splits one or more policies (comma separated) into directives.
func parseCSP(value string) []map[string][]string {
	var policies []map[string][]string
	for _, policy := range strings.Split(value, ",") {
		directives := map[string][]string{}
		for _, part := range strings.Split(policy, ";") {
			fields := strings.Fields(part)
			if len(fields) == 0 {
				continue
			}
			name := strings.ToLower(fields[0])
			if _, dup := directives[name]; !dup {
				directives[name] = fields[1:]
			}
		}
		if len(directives) > 0 {
			policies = append(policies, directives)
		}
	}
	return policies
}

func directive(policies []map[string][]string, name string) (string, bool) {
	for _, p := range policies {
		if v, ok := p[name]; ok {
			return strings.Join(v, " "), true
		}
	}
	return "", false
}

// judgeCSP looks at the sources allowed for scripts and returns the
// directive it judged. With several policies the browser enforces all of
// them, so the strictest one counts.
func judgeCSP(policies []map[string][]string) (Status, string, string) {
	reason, shown := "No script-src or default-src, so scripts may load from anywhere.", ""
	for _, p := range policies {
		key := "script-src"
		sources, ok := p[key]
		if !ok {
			key = "default-src"
			sources, ok = p[key]
		}
		if !ok {
			continue
		}
		shown = strings.TrimSpace(key + " " + strings.Join(sources, " "))
		hasNonce := false
		for _, src := range sources {
			l := strings.ToLower(src)
			if strings.HasPrefix(l, "'nonce-") || strings.HasPrefix(l, "'sha256-") || strings.HasPrefix(l, "'sha384-") || strings.HasPrefix(l, "'sha512-") || l == "'strict-dynamic'" {
				hasNonce = true
			}
		}
		var weak []string
		for _, src := range sources {
			l := strings.ToLower(src)
			switch {
			case l == "'unsafe-inline'" && !hasNonce, l == "'unsafe-eval'", l == "*", l == "http:", l == "https:", l == "data:":
				weak = append(weak, src)
			}
		}
		if len(weak) == 0 {
			return Pass, "Scripts may load only from the sources listed, which stops most cross-site scripting.", shown
		}
		reason = "Scripts are allowed from " + strings.Join(weak, " ") + ", so an injected script can still run. Use nonces or hashes instead."
	}
	return Warn, reason, shown
}

// referrerPolicy returns the policy browsers use: the last value they know.
func referrerPolicy(value string) string {
	known := []string{"no-referrer", "no-referrer-when-downgrade", "origin", "origin-when-cross-origin", "same-origin", "strict-origin", "strict-origin-when-cross-origin", "unsafe-url"}
	last, other := "", ""
	for _, v := range strings.Split(value, ",") {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		other = v
		for _, k := range known {
			if v == k {
				last = v
			}
		}
	}
	if last == "" {
		return other
	}
	return last
}
