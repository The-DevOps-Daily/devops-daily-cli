package check

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DNSOptions struct {
	Server  string   // an IP, optionally with a port; empty: the system resolver
	Types   []string // empty: DefaultDNSTypes
	Timeout time.Duration
}

// DefaultDNSTypes are the record types "check dns" shows without --type.
var DefaultDNSTypes = []string{"A", "AAAA", "MX", "NS", "TXT", "CAA"}

var dnsTypes = map[string]uint16{
	"A": dns.TypeA, "AAAA": dns.TypeAAAA, "CNAME": dns.TypeCNAME, "MX": dns.TypeMX,
	"NS": dns.TypeNS, "TXT": dns.TypeTXT, "CAA": dns.TypeCAA, "SOA": dns.TypeSOA,
}

var typeNotes = map[string]string{
	"A":     "IPv4 addresses. Browsers and other clients connect to these.",
	"AAAA":  "IPv6 addresses.",
	"CNAME": "An alias: this name points to another name, and the other records come from there.",
	"MX":    "The servers that receive mail for this domain, lowest number tried first.",
	"NS":    "The name servers that hold this zone's records.",
	"TXT":   "Free text: SPF, site verification tokens and other settings.",
	"CAA":   "Which certificate authorities may issue certificates. Looked up at this name only; CAs also check parent names.",
	"SOA":   "The zone's main name server, contact address and timers.",
}

type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	TTL   uint32 `json:"ttl"`
	Value string `json:"value"`
}

type DNSDetails struct {
	Name       string   `json:"name"`
	Server     string   `json:"server"`
	Records    []Record `json:"records"`
	CNAMEChain []Record `json:"cnameChain"`
	Errors     []string `json:"errors,omitempty"`
}

// DNS looks up name's records and shows them with their TTLs. The error is
// only for bad input; lookup problems are findings in the report.
func DNS(ctx context.Context, input string, opts DNSOptions) (*Report, error) {
	name, err := domainName(input)
	if err != nil {
		return nil, err
	}
	types, err := parseTypes(opts.Types)
	if err != nil {
		return nil, err
	}
	res, err := newResolver(opts.Server, opts.Timeout)
	if err != nil {
		return nil, err
	}
	d := &DNSDetails{Name: name, Server: res.server, Records: []Record{}, CNAMEChain: []Record{}}
	r := &Report{Check: "dns", Target: name, Unicode: unicodeName(name), Details: d, LearnMore: learnDNS}
	server := Section{Title: "DNS server"}
	server.add("server", Info, "Asked", res.server+" ("+res.source+")", "")
	r.add(server)

	answers := make([]*dns.Msg, len(types))
	errs := make([]error, len(types))
	var wg sync.WaitGroup
	for i, t := range types {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answers[i], errs[i] = res.query(ctx, name, dnsTypes[t])
		}()
	}
	wg.Wait()

	var cnames []*dns.CNAME
	nxdomain := false
	for _, in := range answers {
		if in == nil {
			continue
		}
		nxdomain = nxdomain || in.Rcode == dns.RcodeNameError
		for _, rr := range in.Answer {
			if c, ok := rr.(*dns.CNAME); ok {
				cnames = append(cnames, c)
			}
		}
	}
	chain, loop := cnameChain(name, cnames)
	// A resolver answers a CNAME question with the first alias only, so
	// follow the chain to a name that has data, does not exist or repeats.
	seen := answers
	for len(chain) > 0 && !loop && !nxdomain && len(chain) < maxAliases {
		last := chain[len(chain)-1].Target
		if hasData(seen, last) {
			break
		}
		in, err := res.query(ctx, trimDot(last), dns.TypeA)
		if err != nil {
			break
		}
		seen = append(seen[:len(seen):len(seen)], in)
		if in.Rcode == dns.RcodeNameError {
			nxdomain = true
			break
		}
		before := len(chain)
		for _, rr := range in.Answer {
			if c, ok := rr.(*dns.CNAME); ok {
				cnames = append(cnames, c)
			}
		}
		if chain, loop = cnameChain(name, cnames); len(chain) == before {
			break
		}
	}
	for _, c := range chain {
		d.CNAMEChain = append(d.CNAMEChain, Record{Name: trimDot(c.Hdr.Name), Type: "CNAME", TTL: c.Hdr.Ttl, Value: trimDot(c.Target)})
	}
	alias := Section{Title: "CNAME chain"}
	if len(chain) == 0 {
		alias.add("cname", Info, "", "none: "+name+" is not an alias", "")
	} else {
		for _, c := range d.CNAMEChain {
			alias.add("cname", Info, ttl(c.TTL), c.Name+" -> "+c.Value, "")
		}
		alias.Note = "This name is an alias. The records below come from " + d.CNAMEChain[len(d.CNAMEChain)-1].Value + "."
	}
	r.add(alias)

	result := Section{Title: "Result"}
	switch {
	case loop:
		result.add("cname.loop", Fail, "Alias loop", "the aliases lead back to "+d.CNAMEChain[len(d.CNAMEChain)-1].Value, "The name never resolves. Point one of the CNAMEs at a real name.")
	case len(chain) >= maxAliases:
		result.add("cname.long", Fail, "Alias chain", fmt.Sprintf("%d or more aliases", maxAliases), "Resolvers give up on chains this long. Point the name closer to its target.")
	case nxdomain && len(chain) > 0:
		last := d.CNAMEChain[len(d.CNAMEChain)-1].Value
		result.add("nxdomain", Fail, "Dangling alias", last+" does not exist", "The alias points to a name that is gone. Fix or delete the CNAME: if someone can register the target, they can take over this name.")
	case nxdomain:
		result.add("nxdomain", Fail, "Not found", name+" does not exist (NXDOMAIN)", "No DNS records exist for this name. Check the spelling, or that the zone is set up.")
	}
	if len(result.Findings) > 0 {
		r.add(result)
		return r.finish(), nil
	}

	addresses := 0
	for i, t := range types {
		s := Section{Title: t + " records", Note: typeNotes[t]}
		switch {
		case errs[i] != nil:
			d.Errors = append(d.Errors, t+": "+errs[i].Error())
			s.add(strings.ToLower(t), Fail, "", "lookup failed: "+errs[i].Error(), "")
		case answers[i].Rcode != dns.RcodeSuccess:
			problem := rcodeProblem(answers[i].Rcode).Error()
			d.Errors = append(d.Errors, t+": "+problem)
			s.add(strings.ToLower(t), Fail, "", problem, "")
		default:
			for _, rr := range answers[i].Answer {
				if rr.Header().Rrtype != dnsTypes[t] {
					continue
				}
				record := Record{Name: trimDot(rr.Header().Name), Type: t, TTL: rr.Header().Ttl, Value: rdata(rr)}
				d.Records = append(d.Records, record)
				s.add(strings.ToLower(t), Info, ttl(record.TTL), record.Value, "")
				if t == "A" || t == "AAAA" {
					addresses++
				}
			}
			if len(s.Findings) == 0 {
				status := Info
				if len(opts.Types) > 0 {
					status = Warn
				}
				s.add(strings.ToLower(t), status, "", "none", "")
			}
		}
		r.add(s)
	}
	if len(opts.Types) == 0 && addresses == 0 && len(d.Errors) == 0 {
		s := Section{Title: "Result"}
		s.add("address", Warn, "No address", "no A or AAAA records", "Browsers and most clients cannot connect to this name. That is fine for a name used only for mail or TXT records.")
		r.add(s)
	}
	return r.finish(), nil
}

func parseTypes(values []string) ([]string, error) {
	if len(values) == 0 {
		return DefaultDNSTypes, nil
	}
	var types []string
	for _, value := range values {
		for _, t := range strings.Split(value, ",") {
			t = strings.ToUpper(strings.TrimSpace(t))
			if t == "" {
				continue
			}
			if _, ok := dnsTypes[t]; !ok {
				return nil, fmt.Errorf("unknown record type %q; use A, AAAA, CNAME, MX, NS, TXT, CAA or SOA", t)
			}
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
	}
	if len(types) == 0 {
		return DefaultDNSTypes, nil
	}
	return types, nil
}

// maxAliases is the longest CNAME chain the check follows.
const maxAliases = 16

// cnameChain follows the aliases from name. loop is true when an alias
// points back to a name already in the chain.
func cnameChain(name string, cnames []*dns.CNAME) (chain []*dns.CNAME, loop bool) {
	current := strings.ToLower(dns.Fqdn(name))
	seen := map[string]bool{current: true}
	for len(chain) < maxAliases {
		var next *dns.CNAME
		for _, c := range cnames {
			if strings.EqualFold(c.Hdr.Name, current) {
				next = c
				break
			}
		}
		if next == nil {
			return chain, false
		}
		chain = append(chain, next)
		current = strings.ToLower(dns.Fqdn(next.Target))
		if seen[current] {
			return chain, true
		}
		seen[current] = true
	}
	return chain, false
}

// hasData is true when an answer holds a record other than a CNAME for
// name, which proves the end of the chain exists.
func hasData(answers []*dns.Msg, name string) bool {
	for _, in := range answers {
		if in == nil {
			continue
		}
		for _, rr := range in.Answer {
			if rr.Header().Rrtype != dns.TypeCNAME && strings.EqualFold(rr.Header().Name, name) {
				return true
			}
		}
	}
	return false
}

func rdata(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		return v.A.String()
	case *dns.AAAA:
		return v.AAAA.String()
	case *dns.CNAME:
		return trimDot(v.Target)
	case *dns.MX:
		return fmt.Sprintf("%d %s", v.Preference, trimDot(v.Mx))
	case *dns.NS:
		return trimDot(v.Ns)
	case *dns.TXT:
		return strings.Join(v.Txt, "")
	case *dns.CAA:
		return fmt.Sprintf("%d %s %q", v.Flag, v.Tag, v.Value)
	case *dns.SOA:
		return fmt.Sprintf("%s %s serial %d refresh %s retry %s expire %s minimum %s", trimDot(v.Ns), trimDot(v.Mbox), v.Serial, span(int64(v.Refresh)), span(int64(v.Retry)), span(int64(v.Expire)), span(int64(v.Minttl)))
	}
	header := rr.Header().String()
	return strings.TrimSpace(strings.TrimPrefix(rr.String(), header))
}

func trimDot(s string) string { return strings.TrimSuffix(s, ".") }
