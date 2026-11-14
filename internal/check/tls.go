package check

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"slices"
	"syscall"
	"time"
)

// WarnDays is when an expiring certificate becomes a warning.
const WarnDays = 21

type TLSOptions struct {
	Timeout time.Duration
	Now     func() time.Time
	Roots   *x509.CertPool // nil: the system's trusted roots
}

type CertInfo struct {
	Subject      string    `json:"subject"`
	Issuer       string    `json:"issuer"`
	DNSNames     []string  `json:"dnsNames,omitempty"`
	IPAddresses  []string  `json:"ipAddresses,omitempty"`
	NotBefore    time.Time `json:"notBefore"`
	NotAfter     time.Time `json:"notAfter"`
	SerialNumber string    `json:"serialNumber"`
}

type TLSDetails struct {
	Host          string     `json:"host"`
	Port          string     `json:"port"`
	Address       string     `json:"address,omitempty"`
	Version       string     `json:"version,omitempty"`
	CipherSuite   string     `json:"cipherSuite,omitempty"`
	Certificate   *CertInfo  `json:"certificate,omitempty"`
	DaysLeft      *int       `json:"daysLeft,omitempty"`
	Chain         []CertInfo `json:"chain,omitempty"`
	Trusted       bool       `json:"trusted"`
	TrustError    string     `json:"trustError,omitempty"`
	TrustedRoot   string     `json:"trustedRoot,omitempty"`
	HostnameMatch bool       `json:"hostnameMatch"`
	OCSPStapled   bool       `json:"ocspStapled"`
	Error         string     `json:"error,omitempty"`
}

// TLS connects to target (host or host:port, port 443 by default), reads the
// certificate the server sends and checks it. The error is only for a target
// that cannot be parsed; connection problems are findings in the report.
func TLS(ctx context.Context, target string, opts TLSOptions) (*Report, error) {
	host, port, err := hostPort(target, "443")
	if err != nil {
		return nil, err
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	timeout := timeoutOr(opts.Timeout)
	address := net.JoinHostPort(host, port)
	d := &TLSDetails{Host: host, Port: port}
	r := &Report{Check: "tls", Target: address, Details: d, LearnMore: learnTLS}
	conn := Section{Title: "Connection"}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", address)
	if err != nil {
		d.Error = err.Error()
		conn.add("connect", Fail, "Connect", dialProblem(err, port, timeout), "")
		r.add(conn)
		return r.finish(), nil
	}
	defer raw.Close()
	d.Address = raw.RemoteAddr().String()
	conn.add("address", Info, "Address", d.Address, "The IP address and port this check connected to.")

	// The chain is verified below, against the roots, so that a bad
	// certificate is reported instead of ending the check.
	config := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS10} // #nosec G402
	if net.ParseIP(host) == nil {
		config.ServerName = host
	}
	client := tls.Client(raw, config)
	handshakeCtx, cancelHandshake := context.WithTimeout(ctx, timeout)
	defer cancelHandshake()
	if err := client.HandshakeContext(handshakeCtx); err != nil {
		d.Error = err.Error()
		conn.add("handshake", Fail, "TLS handshake", handshakeProblem(err, timeout), "")
		r.add(conn)
		return r.finish(), nil
	}
	state := client.ConnectionState()
	d.Version = tls.VersionName(state.Version)
	d.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
	switch state.Version {
	case tls.VersionTLS13:
		conn.add("version", Pass, "TLS version", d.Version, "The newest version of TLS.")
	case tls.VersionTLS12:
		conn.add("version", Pass, "TLS version", d.Version, "Still safe. TLS 1.3 is newer and a little faster.")
	default:
		conn.add("version", Fail, "TLS version", d.Version, "Old and unsafe: TLS 1.0 and 1.1 were retired in 2021 and browsers refuse them. Turn on TLS 1.2 and 1.3.")
	}
	if insecureSuite(state.CipherSuite) {
		conn.add("cipher", Warn, "Cipher suite", d.CipherSuite, "The algorithms that encrypt this connection. This one is weak; prefer AES-GCM or ChaCha20 suites.")
	} else {
		conn.add("cipher", Pass, "Cipher suite", d.CipherSuite, "The algorithms that encrypt this connection.")
	}
	r.add(conn)

	peers := state.PeerCertificates
	if len(peers) == 0 {
		cert := Section{Title: "Certificate"}
		cert.add("certificate", Fail, "Certificate", "the server sent no certificate", "")
		r.add(cert)
		return r.finish(), nil
	}
	leaf := peers[0]
	info := certInfo(leaf)
	d.Certificate = &info
	for _, c := range peers {
		d.Chain = append(d.Chain, certInfo(c))
	}
	at := now()

	cert := Section{Title: "Certificate"}
	cert.add("subject", Info, "Subject", info.Subject, "Who the certificate was issued to.")
	names := append(append([]string{}, info.DNSNames...), info.IPAddresses...)
	if len(names) == 0 {
		cert.add("names", Warn, "Names (SANs)", "none", "The certificate lists no host names, so browsers will reject it.")
	} else {
		cert.add("names", Info, "Names (SANs)", list(names, 8), "The host names this certificate is valid for.")
	}
	cert.add("issuer", Info, "Issuer", info.Issuer, "The certificate authority (CA) that signed it.")
	lifetime := int(math.Round(leaf.NotAfter.Sub(leaf.NotBefore).Hours() / 24))
	cert.add("validity", Info, "Valid", fmt.Sprintf("%s to %s (%s in total)", date(leaf.NotBefore), date(leaf.NotAfter), plural(lifetime, "day", "days")), "")

	days := int(math.Floor(leaf.NotAfter.Sub(at).Hours() / 24))
	d.DaysLeft = &days
	renew := fmt.Sprintf("A warning starts at %d days. Certificates are getting shorter (Let's Encrypt: 64 days from February 2027, later 45), so renewal must be automatic.", WarnDays)
	switch {
	case at.Before(leaf.NotBefore):
		cert.add("expiry", Fail, "Expires in", "not valid yet (starts "+date(leaf.NotBefore)+")", "Browsers reject a certificate before its start date. Check the clock on the server that issued it.")
	case days < 0:
		cert.add("expiry", Fail, "Expires in", fmt.Sprintf("expired %s ago (%s)", plural(-days, "day", "days"), date(leaf.NotAfter)), "Browsers block the site with a warning page. Renew the certificate now.")
	case days <= WarnDays:
		cert.add("expiry", Warn, "Expires in", fmt.Sprintf("%s (%s)", plural(days, "day", "days"), date(leaf.NotAfter)), "Renew soon. "+renew)
	default:
		cert.add("expiry", Pass, "Expires in", fmt.Sprintf("%s (%s)", plural(days, "day", "days"), date(leaf.NotAfter)), renew)
	}

	intermediates := x509.NewCertPool()
	for _, c := range peers[1:] {
		intermediates.AddCert(c)
	}
	chains, err := leaf.Verify(x509.VerifyOptions{Roots: opts.Roots, Intermediates: intermediates, CurrentTime: at})
	if err != nil {
		d.TrustError = err.Error()
		cert.add("trusted", Fail, "Trusted", trustProblem(err, peers), "Browsers show a warning page for an untrusted certificate.")
	} else {
		d.Trusted = true
		chain := chains[0]
		d.TrustedRoot = name(chain[len(chain)-1])
		cert.add("trusted", Pass, "Trusted", "yes, chains to "+d.TrustedRoot, "Your system trusts the CA that signed this chain.")
	}
	if err := leaf.VerifyHostname(host); err != nil {
		cert.add("hostname", Fail, "Host name", "does not match "+host, "The certificate is for other names (see Names above), so browsers reject it for "+host+".")
	} else {
		d.HostnameMatch = true
		cert.add("hostname", Pass, "Host name", "matches "+host, "The certificate covers the name you asked for.")
	}
	d.OCSPStapled = len(state.OCSPResponse) > 0
	if d.OCSPStapled {
		cert.add("ocsp", Info, "OCSP stapling", "yes", "The server attaches a recent \"not revoked\" proof from the CA.")
	} else {
		cert.add("ocsp", Info, "OCSP stapling", "no", "No revocation proof is attached. That is common now: browsers rarely ask, and Let's Encrypt ended OCSP in 2025.")
	}
	r.add(cert)

	chain := Section{Title: "Chain", Note: "The certificates the server sent, its own first. Each is signed by the next one; the root comes from your system."}
	for i, c := range peers {
		chain.add(fmt.Sprintf("chain.%d", i+1), Info, fmt.Sprint(i+1), fmt.Sprintf("%s, issued by %s, expires %s", name(c), issuerName(c), c.NotAfter.UTC().Format("2006-01-02")), "")
	}
	if d.Trusted {
		chain.add("chain.root", Info, "root", d.TrustedRoot+" (from your system)", "")
	}
	r.add(chain)
	return r.finish(), nil
}

func certInfo(c *x509.Certificate) CertInfo {
	info := CertInfo{Subject: name(c), Issuer: issuerName(c), DNSNames: c.DNSNames, NotBefore: c.NotBefore.UTC(), NotAfter: c.NotAfter.UTC(), SerialNumber: fmt.Sprintf("%X", c.SerialNumber)}
	if len(c.Subject.Organization) > 0 && c.Subject.CommonName != "" {
		info.Subject += " (" + c.Subject.Organization[0] + ")"
	}
	if len(c.Issuer.Organization) > 0 && c.Issuer.CommonName != "" {
		info.Issuer += " (" + c.Issuer.Organization[0] + ")"
	}
	for _, ip := range c.IPAddresses {
		info.IPAddresses = append(info.IPAddresses, ip.String())
	}
	return info
}

func name(c *x509.Certificate) string {
	switch {
	case c.Subject.CommonName != "":
		return c.Subject.CommonName
	case len(c.DNSNames) > 0:
		return c.DNSNames[0]
	case len(c.Subject.Organization) > 0:
		return c.Subject.Organization[0]
	}
	return c.Subject.String()
}

func issuerName(c *x509.Certificate) string {
	switch {
	case c.Issuer.CommonName != "":
		return c.Issuer.CommonName
	case len(c.Issuer.Organization) > 0:
		return c.Issuer.Organization[0]
	}
	return c.Issuer.String()
}

func date(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") }

func selfSigned(c *x509.Certificate) bool {
	return bytes.Equal(c.RawIssuer, c.RawSubject) && c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil
}

func insecureSuite(id uint16) bool {
	return slices.ContainsFunc(tls.InsecureCipherSuites(), func(s *tls.CipherSuite) bool { return s.ID == id })
}

func trustProblem(err error, peers []*x509.Certificate) string {
	var invalid x509.CertificateInvalidError
	var unknown x509.UnknownAuthorityError
	switch {
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		return "no, a certificate in the chain has expired or is not valid yet"
	case selfSigned(peers[0]):
		return "no, the certificate is self-signed, so no trusted CA vouches for it"
	case errors.As(err, &unknown) && len(peers) == 1:
		return "no, the server sent only its own certificate and your system does not know the CA that signed it (a missing intermediate?)"
	case errors.As(err, &unknown):
		return "no, the chain does not lead to a root your system trusts"
	}
	return "no, " + err.Error()
}

func dialProblem(err error, port string, timeout time.Duration) string {
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return "could not find the host: the name does not resolve to an address"
	case errors.As(err, &dnsErr):
		return "could not look up the host: " + dnsErr.Err
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection refused: nothing is listening on port " + port
	case isTimeout(err):
		return fmt.Sprintf("no answer within %s (a firewall may be dropping the traffic)", timeout)
	}
	return err.Error()
}

func handshakeProblem(err error, timeout time.Duration) string {
	var record tls.RecordHeaderError
	switch {
	case errors.As(err, &record):
		return "failed: the server did not answer with TLS (is this a plain HTTP port?)"
	case isTimeout(err):
		return fmt.Sprintf("failed: no TLS answer within %s", timeout)
	}
	return "failed: " + err.Error()
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout()
}
