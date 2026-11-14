package check

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// hostPort reads "host", "host:port", "[::1]:port" or a pasted URL and
// returns the host (lower case, no trailing dot) and the port.
func hostPort(input, defaultPort string) (string, string, error) {
	s := strings.TrimSpace(input)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	host, port := s, defaultPort
	switch {
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
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if net.ParseIP(host) == nil && !validName(host) {
		return "", "", fmt.Errorf("%q is not a valid host name", host)
	}
	return host, port, nil
}

// domainName reads a domain, a pasted URL or an email address and returns
// the domain in lower case without a trailing dot.
func domainName(input string) (string, error) {
	s := strings.TrimSpace(input)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.ToLower(s), ".")
	if s == "" {
		return "", errors.New("give a domain name, like example.com")
	}
	if net.ParseIP(s) != nil {
		return "", fmt.Errorf("%s is an IP address; give a domain name, like example.com", s)
	}
	if !validName(s) {
		return "", fmt.Errorf("%q is not a valid domain name", input)
	}
	return s, nil
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
