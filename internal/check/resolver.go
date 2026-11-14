package check

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// fallbackServer is used only when the system resolver cannot be read
// (for example on Windows, which has no /etc/resolv.conf).
const fallbackServer = "1.1.1.1:53"

type resolver struct {
	server  string
	source  string
	timeout time.Duration
}

// newResolver uses server (an IP, optionally with a port) or, when it is
// empty, the first name server in /etc/resolv.conf.
func newResolver(server string, timeout time.Duration) (*resolver, error) {
	r := &resolver{timeout: timeoutOr(timeout)}
	if server != "" {
		address, err := serverAddress(server)
		if err != nil {
			return nil, err
		}
		r.server, r.source = address, "chosen with --server"
		return r, nil
	}
	if config, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(config.Servers) > 0 {
		r.server, r.source = net.JoinHostPort(config.Servers[0], config.Port), "your system resolver"
		return r, nil
	}
	r.server, r.source = fallbackServer, "Cloudflare; your system resolver could not be read, so pick one with --server"
	return r, nil
}

func serverAddress(value string) (string, error) {
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), nil
	}
	host, port, err := net.SplitHostPort(value)
	if n, perr := strconv.Atoi(port); err == nil && net.ParseIP(host) != nil && perr == nil && n > 0 && n < 65536 {
		return net.JoinHostPort(host, port), nil
	}
	return "", fmt.Errorf("--server must be an IP address, like 1.1.1.1 or 8.8.8.8:53 (got %q)", value)
}

// query asks one question, over UDP first and again over TCP when the
// answer does not fit.
func (r *resolver) query(ctx context.Context, name string, qtype uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.SetEdns0(1232, false)
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	c := &dns.Client{Net: "udp", UDPSize: 1232, Timeout: r.timeout}
	in, _, err := c.ExchangeContext(ctx, m, r.server)
	if err == nil && in.Truncated {
		c.Net = "tcp"
		in, _, err = c.ExchangeContext(ctx, m, r.server)
	}
	if err != nil {
		if isTimeout(err) {
			return nil, fmt.Errorf("no answer from %s within %s", r.server, r.timeout)
		}
		return nil, err
	}
	return in, nil
}

// txt returns the TXT records at name, each one's strings joined. A name
// that does not exist gives no records and no error.
func (r *resolver) txt(ctx context.Context, name string) ([]string, error) {
	in, err := r.query(ctx, name, dns.TypeTXT)
	if err != nil {
		return nil, err
	}
	if in.Rcode != dns.RcodeSuccess && in.Rcode != dns.RcodeNameError {
		return nil, rcodeProblem(in.Rcode)
	}
	var values []string
	for _, rr := range in.Answer {
		if t, ok := rr.(*dns.TXT); ok {
			values = append(values, strings.Join(t.Txt, ""))
		}
	}
	return values, nil
}

func rcodeProblem(rcode int) error {
	switch rcode {
	case dns.RcodeServerFailure:
		return fmt.Errorf("the DNS server failed to answer (SERVFAIL; often a broken DNSSEC signature or a dead name server)")
	case dns.RcodeRefused:
		return fmt.Errorf("the DNS server refused the question (REFUSED; it may not answer for you)")
	}
	return fmt.Errorf("the DNS server answered %s", dns.RcodeToString[rcode])
}

// withPrefix keeps the records that start with prefix, ignoring case.
func withPrefix(records []string, prefix string) []string {
	var out []string
	for _, record := range records {
		value := strings.TrimSpace(record)
		if len(value) >= len(prefix) && strings.EqualFold(value[:len(prefix)], prefix) && (len(value) == len(prefix) || value[len(prefix)] == ' ' || value[len(prefix)] == ';') {
			out = append(out, value)
		}
	}
	return out
}
