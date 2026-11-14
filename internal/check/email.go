package check

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// SPFLookupLimit is the most DNS lookups an SPF check may need (RFC 7208).
const SPFLookupLimit = 10

// DKIMSelectors are the selectors "check email" tries. DKIM keys live at
// <selector>._domainkey.<domain> and there is no way to list them.
var DKIMSelectors = []string{"default", "google", "selector1", "selector2", "k1", "s1", "mail", "dkim"}

type EmailOptions struct {
	Server    string
	Selectors []string // tried after DKIMSelectors
	Timeout   time.Duration
}

type MX struct {
	Preference uint16 `json:"preference"`
	Host       string `json:"host"`
}

type SPFDetails struct {
	Records  []string `json:"records"`
	Lookups  int      `json:"lookups"`
	All      string   `json:"all,omitempty"`
	Problems []string `json:"problems,omitempty"`
}

type DMARCDetails struct {
	Records []string          `json:"records"`
	Tags    map[string]string `json:"tags,omitempty"`
}

type DKIMKey struct {
	Selector string `json:"selector"`
	Found    bool   `json:"found"`
	KeyType  string `json:"keyType,omitempty"`
	Bits     int    `json:"bits,omitempty"`
	Revoked  bool   `json:"revoked,omitempty"`
	Error    string `json:"error,omitempty"`
}

type EmailDetails struct {
	Domain string       `json:"domain"`
	Server string       `json:"server"`
	MX     []MX         `json:"mx"`
	SPF    SPFDetails   `json:"spf"`
	DMARC  DMARCDetails `json:"dmarc"`
	DKIM   []DKIMKey    `json:"dkim"`
	MTASTS string       `json:"mtaSts,omitempty"`
	TLSRPT string       `json:"tlsRpt,omitempty"`
}

// Email checks the DNS records that decide whether mail from and to domain
// is trusted: MX, SPF, DMARC, DKIM, MTA-STS and TLS-RPT. The error is only
// for bad input.
func Email(ctx context.Context, input string, opts EmailOptions) (*Report, error) {
	domain, err := domainName(input)
	if err != nil {
		return nil, err
	}
	res, err := newResolver(opts.Server, opts.Timeout)
	if err != nil {
		return nil, err
	}
	d := &EmailDetails{Domain: domain, Server: res.server, MX: []MX{}, SPF: SPFDetails{Records: []string{}}, DMARC: DMARCDetails{Records: []string{}}, DKIM: []DKIMKey{}}
	r := &Report{Check: "email", Target: domain, Details: d, LearnMore: learnEmail}
	server := Section{Title: "DNS server"}
	server.add("server", Info, "Asked", res.server+" ("+res.source+")", "")
	r.add(server)

	mx := Section{Title: "Mail servers (MX)", Note: "The servers that receive mail for this domain. Senders try the lowest number first."}
	in, err := res.query(ctx, domain, dns.TypeMX)
	switch {
	case err != nil:
		mx.add("mx", Fail, "", "lookup failed: "+err.Error(), "")
	case in.Rcode == dns.RcodeNameError:
		mx.add("mx", Fail, "", domain+" does not exist (NXDOMAIN)", "There are no DNS records for this name. Check the spelling.")
		r.add(mx)
		return r.finish(), nil
	case in.Rcode != dns.RcodeSuccess:
		mx.add("mx", Fail, "", rcodeProblem(in.Rcode).Error(), "")
	default:
		for _, rr := range in.Answer {
			if m, ok := rr.(*dns.MX); ok {
				d.MX = append(d.MX, MX{Preference: m.Preference, Host: trimDot(m.Mx)})
			}
		}
		sort.SliceStable(d.MX, func(i, j int) bool { return d.MX[i].Preference < d.MX[j].Preference })
		switch {
		case len(d.MX) == 1 && (d.MX[0].Host == "" || d.MX[0].Host == "."):
			mx.add("mx", Info, "", "null MX: this domain accepts no mail", "The owner says no mail should be sent here (RFC 7505).")
		case len(d.MX) == 0:
			mx.add("mx", Warn, "", "none", "Without MX records, mail for this domain goes to its A record, if there is one, or bounces.")
		default:
			for _, m := range d.MX {
				mx.add("mx", Info, fmt.Sprint(m.Preference), m.Host, "")
			}
		}
	}
	r.add(mx)

	r.add(checkSPF(ctx, res, domain, &d.SPF))
	r.add(checkDMARC(ctx, res, domain, &d.DMARC))
	r.add(checkDKIM(ctx, res, domain, opts.Selectors, &d.DKIM))
	r.add(checkTransport(ctx, res, domain, d))
	return r.finish(), nil
}

func checkSPF(ctx context.Context, res *resolver, domain string, d *SPFDetails) Section {
	s := Section{Title: "SPF", Note: "Lists the servers allowed to send mail as this domain. Receivers compare it with the sending server."}
	records, err := res.txt(ctx, domain)
	if err != nil {
		s.add("spf", Fail, "Record", "lookup failed: "+err.Error(), "")
		return s
	}
	d.Records = append(d.Records, withPrefix(records, "v=spf1")...)
	switch len(d.Records) {
	case 0:
		s.add("spf", Missing, "Record", "missing", "Without SPF, receivers cannot tell which servers may send as this domain, and more of your mail lands in spam.")
		return s
	case 1:
		s.add("spf", Pass, "Record", d.Records[0], "")
	default:
		s.add("spf", Fail, "Record", fmt.Sprintf("%d SPF records", len(d.Records)), "Receivers treat more than one SPF record as an error (permerror), so SPF fails. Merge them into one.")
		for _, record := range d.Records {
			s.add("spf", Info, "", record, "")
		}
		return s
	}

	w := &spfWalk{res: res, path: map[string]bool{domain: true}}
	all, via := w.record(ctx, d.Records[0], 0)
	d.Lookups, d.All, d.Problems = w.lookups, all, w.problems
	lookupNote := "Each include, a, mx, ptr, exists and redirect costs one DNS lookup, nested ones too. Over 10 and SPF fails."
	if w.lookups > SPFLookupLimit {
		s.add("spf.lookups", Fail, "DNS lookups", fmt.Sprintf("more than %d", SPFLookupLimit), lookupNote+" Remove includes you no longer use, or replace some with ip4/ip6.")
	} else {
		s.add("spf.lookups", Pass, "DNS lookups", fmt.Sprintf("%d of %d", w.lookups, SPFLookupLimit), lookupNote)
	}
	for _, problem := range w.problems {
		s.add("spf.include", Fail, "Problem", problem, "")
	}
	if w.ptr {
		s.add("spf.ptr", Warn, "ptr", "uses the ptr mechanism", "ptr is deprecated: slow, unreliable and some receivers skip it. Use ip4, ip6 or include.")
	}
	value := all
	if via != "" {
		value += " (from redirect=" + via + ")"
	}
	switch strings.TrimPrefix(all, "+") {
	case "-all":
		s.add("spf.all", Pass, "Other servers", value, "Fail: mail from any server not listed fails SPF.")
	case "~all":
		s.add("spf.all", Pass, "Other servers", value, "Softfail: mail from servers not listed is marked suspicious. Fine together with a DMARC policy.")
	case "?all":
		s.add("spf.all", Warn, "Other servers", value, "Neutral: says nothing about servers not listed, so SPF gives no protection. Use ~all or -all.")
	case "all":
		s.add("spf.all", Fail, "Other servers", value, "Anyone may send mail as this domain. Use ~all or -all.")
	default:
		s.add("spf.all", Warn, "Other servers", "no all at the end", "Servers not listed get neutral, which gives no protection. End the record with ~all or -all.")
	}
	return s
}

type spfWalk struct {
	res      *resolver
	lookups  int
	problems []string
	ptr      bool
	path     map[string]bool
}

// record counts the lookups in one SPF record and its includes and returns
// its "all" term, following redirect= when the record has none.
func (w *spfWalk) record(ctx context.Context, record string, depth int) (all, via string) {
	redirect := ""
	for _, term := range strings.Fields(record)[1:] {
		qualifier := ""
		if strings.ContainsRune("+-~?", rune(term[0])) {
			qualifier, term = term[:1], term[1:]
		}
		name, arg := term, ""
		if i := strings.IndexAny(term, ":="); i >= 0 {
			name, arg = term[:i], strings.ToLower(term[i+1:])
		}
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[:i]
		}
		switch strings.ToLower(name) {
		case "all":
			all = qualifier + "all"
		case "include":
			w.lookups++
			w.include(ctx, arg, depth)
		case "a", "mx", "exists":
			w.lookups++
		case "ptr":
			w.lookups++
			w.ptr = true
		case "redirect":
			w.lookups++
			redirect = arg
		}
	}
	if all != "" || redirect == "" || strings.Contains(redirect, "%") {
		return all, ""
	}
	target, ok := w.fetch(ctx, "redirect="+redirect, redirect, depth)
	if !ok {
		return "", redirect
	}
	all, _ = w.record(ctx, target, depth+1)
	delete(w.path, redirect)
	return all, redirect
}

func (w *spfWalk) include(ctx context.Context, domain string, depth int) {
	if strings.Contains(domain, "%") {
		return // a macro, only known when mail arrives
	}
	if record, ok := w.fetch(ctx, "include:"+domain, domain, depth); ok {
		w.record(ctx, record, depth+1)
		delete(w.path, domain)
	}
}

// fetch reads the SPF record a term points to. It stops once the count is
// over the limit, since the result is already known.
func (w *spfWalk) fetch(ctx context.Context, term, domain string, depth int) (string, bool) {
	if w.lookups > SPFLookupLimit || depth >= SPFLookupLimit {
		return "", false
	}
	if w.path[domain] {
		w.problems = append(w.problems, term+" loops back to a record that includes it")
		return "", false
	}
	records, err := w.res.txt(ctx, domain)
	if err != nil {
		w.problems = append(w.problems, term+": lookup failed: "+err.Error())
		return "", false
	}
	spf := withPrefix(records, "v=spf1")
	switch len(spf) {
	case 0:
		w.problems = append(w.problems, term+" has no SPF record, so receivers return an error (permerror)")
		return "", false
	case 1:
		w.path[domain] = true
		return spf[0], true
	}
	w.problems = append(w.problems, fmt.Sprintf("%s has %d SPF records, so receivers return an error (permerror)", term, len(spf)))
	return "", false
}

func checkDMARC(ctx context.Context, res *resolver, domain string, d *DMARCDetails) Section {
	s := Section{Title: "DMARC", Note: "Tells receivers what to do with mail that fails both SPF and DKIM for this domain, and where to send reports."}
	records, err := res.txt(ctx, "_dmarc."+domain)
	if err != nil {
		s.add("dmarc", Fail, "Record", "lookup failed: "+err.Error(), "")
		return s
	}
	d.Records = append(d.Records, withPrefix(records, "v=DMARC1")...)
	switch len(d.Records) {
	case 0:
		meaning := "No DMARC record at _dmarc." + domain + ": spoofed mail is not blocked and you get no reports. Start with v=DMARC1; p=none; rua=mailto:<you>."
		if strings.Count(domain, ".") > 1 {
			meaning += " For a subdomain, the parent domain's record may apply."
		}
		s.add("dmarc", Missing, "Record", "missing", meaning)
		return s
	case 1:
		s.add("dmarc", Pass, "Record", d.Records[0], "")
	default:
		s.add("dmarc", Fail, "Record", fmt.Sprintf("%d DMARC records", len(d.Records)), "With more than one record, receivers ignore DMARC for this domain. Keep one.")
		return s
	}
	tags := tagList(d.Records[0])
	d.Tags = tags

	switch strings.ToLower(tags["p"]) {
	case "reject":
		s.add("dmarc.p", Pass, "Policy (p)", "reject", "Mail that fails is refused.")
	case "quarantine":
		s.add("dmarc.p", Pass, "Policy (p)", "quarantine", "Mail that fails goes to spam.")
	case "none":
		s.add("dmarc.p", Warn, "Policy (p)", "none", "Monitoring only: mail that fails is still delivered. Move to quarantine, then reject, once the reports look clean.")
	case "":
		s.add("dmarc.p", Fail, "Policy (p)", "missing", "p= is required. Without it receivers ignore the record.")
	default:
		s.add("dmarc.p", Fail, "Policy (p)", tags["p"], "Not a valid policy; use none, quarantine or reject.")
	}
	if sp, ok := tags["sp"]; ok {
		s.add("dmarc.sp", Info, "Subdomains (sp)", sp, "The policy for subdomains.")
	}
	if np, ok := tags["np"]; ok {
		s.add("dmarc.np", Info, "Missing names (np)", np, "The policy for subdomains that do not exist, a common spoofing target.")
	}
	if pct, ok := tags["pct"]; ok {
		if n, err := strconv.Atoi(pct); err == nil && n == 100 {
			s.add("dmarc.pct", Info, "Percent (pct)", "100", "The policy applies to all failing mail. The updated DMARC standard (DMARCbis) drops pct, so you can delete it.")
		} else {
			s.add("dmarc.pct", Warn, "Percent (pct)", pct, "Only that share of failing mail gets the policy. The updated DMARC standard (DMARCbis) drops pct: receivers that follow it apply the full policy. Use t=y while testing instead.")
		}
	} else {
		s.add("dmarc.pct", Info, "Percent (pct)", "not set (all mail)", "The policy applies to all failing mail.")
	}
	if strings.EqualFold(tags["t"], "y") {
		s.add("dmarc.t", Warn, "Testing (t)", "y", "Testing mode: receivers send reports but do not apply the policy.")
	}
	if rua := tags["rua"]; rua != "" {
		s.add("dmarc.rua", Pass, "Reports (rua)", rua, "Daily summary reports of who sends mail as you go here.")
	} else {
		s.add("dmarc.rua", Warn, "Reports (rua)", "not set", "Without rua you get no reports, so you cannot see who sends mail as you or whether it passes.")
	}
	return s
}

func checkDKIM(ctx context.Context, res *resolver, domain string, extra []string, keys *[]DKIMKey) Section {
	s := Section{Title: "DKIM", Note: "Public keys that receivers use to check the signature on your mail. Each key sits under a selector name."}
	selectors := append([]string{}, DKIMSelectors...)
	for _, sel := range extra {
		sel = strings.ToLower(strings.TrimSpace(sel))
		if sel != "" && !slices.Contains(selectors, sel) {
			selectors = append(selectors, sel)
		}
	}
	results := make([]DKIMKey, len(selectors))
	var wg sync.WaitGroup
	for i, sel := range selectors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = dkimKey(ctx, res, sel, domain)
		}()
	}
	wg.Wait()
	*keys = results

	var missing []string
	found := 0
	for _, k := range results {
		label := k.Selector
		switch {
		case k.Error != "":
			s.add("dkim", Warn, label, "lookup failed: "+k.Error, "")
		case !k.Found:
			missing = append(missing, k.Selector)
		case k.Revoked:
			found++
			s.add("dkim", Info, label, "revoked (empty key)", "This selector was switched off on purpose.")
		case k.Bits == 0:
			found++
			s.add("dkim", Warn, label, "key not readable ("+k.KeyType+")", "The p= value is not a key this check can read.")
		case k.KeyType == "rsa" && k.Bits < 1024:
			found++
			s.add("dkim", Fail, label, fmt.Sprintf("RSA %d-bit", k.Bits), "Too short: receivers ignore keys under 1024 bits. Make a 2048-bit key.")
		case k.KeyType == "rsa" && k.Bits < 2048:
			found++
			s.add("dkim", Warn, label, fmt.Sprintf("RSA %d-bit", k.Bits), "Works, but 2048 bits is the current advice.")
		case k.KeyType == "rsa":
			found++
			s.add("dkim", Pass, label, fmt.Sprintf("RSA %d-bit", k.Bits), "")
		default:
			found++
			s.add("dkim", Pass, label, "Ed25519", "Not every receiver checks Ed25519 yet, so also sign with an RSA key.")
		}
	}
	if len(missing) > 0 {
		s.add("dkim.missing", Info, "Not found", strings.Join(missing, ", "), "")
	}
	if found == 0 {
		s.add("dkim.none", Warn, "Result", "no key at the common selectors", "DKIM may still use another selector. Open a mail from this domain, find s= in its DKIM-Signature header, then run with --selector <s>.")
	}
	return s
}

func dkimKey(ctx context.Context, res *resolver, selector, domain string) DKIMKey {
	key := DKIMKey{Selector: selector}
	records, err := res.txt(ctx, selector+"._domainkey."+domain)
	if err != nil {
		key.Error = err.Error()
		return key
	}
	for _, record := range records {
		tags := tagList(record)
		p, hasKey := tags["p"]
		if !hasKey && !strings.EqualFold(tags["v"], "DKIM1") {
			continue // not a DKIM record (a wildcard TXT, for example)
		}
		key.Found = true
		key.KeyType = strings.ToLower(tags["k"])
		if key.KeyType == "" {
			key.KeyType = "rsa"
		}
		p = strings.Join(strings.Fields(p), "")
		if p == "" {
			key.Revoked = true
			return key
		}
		der, err := base64.StdEncoding.DecodeString(p)
		if err != nil {
			return key
		}
		switch key.KeyType {
		case "rsa":
			if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
				if rsaKey, ok := pub.(*rsa.PublicKey); ok {
					key.Bits = rsaKey.N.BitLen()
				}
			} else if rsaKey, err := x509.ParsePKCS1PublicKey(der); err == nil {
				key.Bits = rsaKey.N.BitLen()
			}
		case "ed25519":
			if len(der) == ed25519.PublicKeySize {
				key.Bits = 256
			}
		}
		return key
	}
	return key
}

func checkTransport(ctx context.Context, res *resolver, domain string, d *EmailDetails) Section {
	s := Section{Title: "MTA-STS and TLS-RPT", Note: "Optional. They make other mail servers use TLS when they deliver to you, and report when that fails."}
	policy := "https://mta-sts." + domain + "/.well-known/mta-sts.txt"
	if records, err := res.txt(ctx, "_mta-sts."+domain); err != nil {
		s.add("mta-sts", Warn, "MTA-STS", "lookup failed: "+err.Error(), "")
	} else if sts := withPrefix(records, "v=STSv1"); len(sts) > 0 {
		d.MTASTS = sts[0]
		s.add("mta-sts", Pass, "MTA-STS", sts[0], "Senders fetch the policy from "+policy+" (not downloaded by this check).")
	} else {
		s.add("mta-sts", Info, "MTA-STS", "not set", "Without it, a sender may deliver in plain text when TLS to your servers fails. The policy would live at "+policy+".")
	}
	if records, err := res.txt(ctx, "_smtp._tls."+domain); err != nil {
		s.add("tls-rpt", Warn, "TLS-RPT", "lookup failed: "+err.Error(), "")
	} else if rpt := withPrefix(records, "v=TLSRPTv1"); len(rpt) > 0 {
		d.TLSRPT = rpt[0]
		s.add("tls-rpt", Pass, "TLS-RPT", rpt[0], "Senders report TLS problems delivering to you to the rua address.")
	} else {
		s.add("tls-rpt", Info, "TLS-RPT", "not set", "Lets senders report TLS problems when they deliver to you.")
	}
	return s
}

// tagList parses "k=v; k2=v2" records (DMARC, DKIM). Keys are lower case.
func tagList(record string) map[string]string {
	tags := map[string]string{}
	for _, part := range strings.Split(record, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, dup := tags[key]; !dup && key != "" {
			tags[key] = strings.TrimSpace(value)
		}
	}
	return tags
}
