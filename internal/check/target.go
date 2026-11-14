package check

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/idna"
)

// hostPort reads "host", "host:port", "[::1]:port" or a pasted URL and
// returns the host in ASCII (lower case, no trailing dot) and the port.
func hostPort(input, defaultPort string) (string, string, error) {
	s := strings.TrimSpace(input)
	host, port := s, defaultPort
	switch {
	case strings.Contains(s, "://") || strings.ContainsAny(s, "/?#"):
		raw := s
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return "", "", fmt.Errorf("%q is not a host or host:port", input)
		}
		host = u.Hostname()
		if u.Port() != "" {
			port = u.Port()
		}
	case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
		host = s[1 : len(s)-1]
	case strings.HasPrefix(s, "[") || strings.Count(s, ":") == 1:
		h, p, err := net.SplitHostPort(s)
		if err != nil {
			return "", "", fmt.Errorf("%q is not a host or host:port", input)
		}
		host, port = h, p
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", "", fmt.Errorf("%q is not a valid port", port)
	}
	host, err := asciiName(host)
	if err != nil {
		return "", "", err
	}
	return host, port, nil
}

// domainName reads a domain, an email address or a pasted URL and returns
// the domain in ASCII, lower case and without a trailing dot.
func domainName(input string) (string, error) {
	s := strings.TrimSpace(input)
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("%q is not a valid domain name", input)
		}
		s = u.Hostname()
	case strings.Contains(s, "@"):
		s = s[strings.LastIndex(s, "@")+1:]
	case strings.ContainsAny(s, "/?#"):
		u, err := url.Parse("https://" + s)
		if err != nil {
			return "", fmt.Errorf("%q is not a valid domain name", input)
		}
		s = u.Hostname()
	case strings.Count(s, ":") == 1:
		if h, _, err := net.SplitHostPort(s); err == nil {
			s = h
		}
	}
	if s == "" {
		return "", errors.New("give a domain name, like example.com")
	}
	if net.ParseIP(s) != nil {
		return "", fmt.Errorf("%s is an IP address; give a domain name, like example.com", s)
	}
	return asciiName(s)
}

// asciiName lowers host and drops a trailing dot. International names
// (bücher.de) become their ASCII form (xn--bcher-kva.de), the one DNS,
// TLS and certificates use. IP addresses are returned as they are.
func asciiName(host string) (string, error) {
	host = strings.TrimSuffix(host, ".")
	if net.ParseIP(host) != nil {
		return strings.ToLower(host), nil
	}
	for _, r := range host {
		if r >= 0x80 {
			ascii, err := idna.Lookup.ToASCII(host)
			if err != nil {
				return "", fmt.Errorf("%q is not a valid international domain name", host)
			}
			host = ascii
			break
		}
	}
	host = strings.ToLower(host)
	if !validName(host) {
		return "", fmt.Errorf("%q is not a valid host name", host)
	}
	return host, nil
}

// unicodeName returns the Unicode form of an xn-- name, or "" when the
// name has none, so reports can show both.
func unicodeName(host string) string {
	if !strings.Contains(host, "xn--") {
		return ""
	}
	u, err := idna.Lookup.ToUnicode(host)
	if err != nil || u == host {
		return ""
	}
	return u
}

// validName accepts DNS names made of letters, digits, hyphens and
// underscores (for names like _dmarc.example.com).
func validName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
	}
	return true
}
