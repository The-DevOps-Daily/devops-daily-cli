package check

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, time.November, 14, 12, 0, 0, 0, time.UTC)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root CA", Organization: []string{"DevOps Daily Tests"}},
		NotBefore:             testNow.AddDate(-1, 0, 0),
		NotAfter:              testNow.AddDate(5, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: cert, key: key}
}

func (ca *testCA) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

// leaf issues a server certificate. A nil ca makes it self-signed.
func leaf(t *testing.T, ca *testCA, notAfter time.Time, dnsNames []string, ips ...net.IP) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "server.test"},
		NotBefore:    notAfter.AddDate(0, 0, -90),
		NotAfter:     notAfter,
		DNSNames:     dnsNames,
		IPAddresses:  ips,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	parent, signer := template, any(key)
	if ca != nil {
		parent, signer = ca.cert, ca.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func tlsServer(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv.Listener.Addr().String()
}

func now() time.Time { return testNow }

var localhost = net.ParseIP("127.0.0.1")

func TestTLSGoodCertificate(t *testing.T) {
	ca := newTestCA(t)
	addr := tlsServer(t, leaf(t, ca, testNow.AddDate(0, 0, 60), []string{"server.test"}, localhost))
	r, err := TLS(context.Background(), addr, TLSOptions{Roots: ca.pool(), Now: now, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 {
		t.Fatalf("exit code = %d, report = %+v", r.ExitCode, r.Sections)
	}
	d := r.Details.(*TLSDetails)
	if !d.Trusted || !d.HostnameMatch || d.DaysLeft == nil || *d.DaysLeft != 60 || d.TrustedRoot != "Test Root CA" {
		t.Fatalf("details = %+v", d)
	}
	if d.Version != "TLS 1.3" || d.CipherSuite == "" || d.OCSPStapled {
		t.Fatalf("details = %+v", d)
	}
	if f := finding(t, r, "expiry"); f.Status != Pass || !strings.HasPrefix(f.Value, "60 days") {
		t.Fatalf("expiry = %+v", f)
	}
	if f := finding(t, r, "names"); f.Value != "server.test, 127.0.0.1" {
		t.Fatalf("names = %+v", f)
	}
	if r.LearnMore != learnTLS || !hasFinding(r, "chain.root") {
		t.Fatalf("report = %+v", r)
	}
}

func TestTLSExpiry(t *testing.T) {
	ca := newTestCA(t)
	notAfter := testNow.AddDate(0, 0, 30)
	addr := tlsServer(t, leaf(t, ca, notAfter, nil, localhost))
	cases := []struct {
		at     time.Time
		status Status
		code   int
		value  string
	}{
		{notAfter.AddDate(0, 0, -22), Pass, 0, "22 days"},
		{notAfter.AddDate(0, 0, -21), Warn, 1, "21 days"},
		{notAfter.Add(-36 * time.Hour), Warn, 1, "1 day"},
		{notAfter.AddDate(0, 0, 3), Fail, 2, "expired 3 days ago"},
		{notAfter.AddDate(0, 0, -100), Fail, 2, "not valid yet"},
	}
	for _, c := range cases {
		at := c.at
		r, err := TLS(context.Background(), addr, TLSOptions{Roots: ca.pool(), Now: func() time.Time { return at }})
		if err != nil {
			t.Fatal(err)
		}
		f := finding(t, r, "expiry")
		if f.Status != c.status || r.ExitCode != c.code || !strings.HasPrefix(f.Value, c.value) {
			t.Errorf("at %s: expiry %+v, exit %d; want %s %q exit %d", at, f, r.ExitCode, c.status, c.value, c.code)
		}
	}
}

func TestTLSUntrustedAndWrongName(t *testing.T) {
	ca := newTestCA(t)
	addr := tlsServer(t, leaf(t, ca, testNow.AddDate(0, 0, 60), []string{"server.test"}))

	r, err := TLS(context.Background(), addr, TLSOptions{Roots: x509.NewCertPool(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 2 {
		t.Fatalf("exit code = %d", r.ExitCode)
	}
	if f := finding(t, r, "trusted"); f.Status != Fail || !strings.Contains(f.Value, "intermediate") {
		t.Fatalf("trusted = %+v", f)
	}
	if f := finding(t, r, "hostname"); f.Status != Fail || !strings.Contains(f.Value, "127.0.0.1") {
		t.Fatalf("hostname = %+v", f)
	}
}

func TestTLSSelfSignedAndStapled(t *testing.T) {
	cert := leaf(t, nil, testNow.AddDate(0, 0, 60), nil, localhost)
	cert.OCSPStaple = []byte{0x30, 0x03, 0x0a, 0x01, 0x00}
	addr := tlsServer(t, cert)
	r, err := TLS(context.Background(), addr, TLSOptions{Roots: x509.NewCertPool(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "trusted"); f.Status != Fail || !strings.Contains(f.Value, "self-signed") {
		t.Fatalf("trusted = %+v", f)
	}
	if f := finding(t, r, "ocsp"); f.Value != "yes" {
		t.Fatalf("ocsp = %+v", f)
	}
	if f := finding(t, r, "hostname"); f.Status != Pass {
		t.Fatalf("hostname = %+v", f)
	}
}

func TestTLSConnectionProblems(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().String()
	ln.Close()
	r, err := TLS(context.Background(), closed, TLSOptions{Now: now, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "connect"); f.Status != Fail || r.ExitCode != 2 || !strings.Contains(f.Value, "refused") {
		t.Fatalf("connect = %+v, exit %d", f, r.ExitCode)
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer plain.Close()
	r, err = TLS(context.Background(), plain.Listener.Addr().String(), TLSOptions{Now: now, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "handshake"); f.Status != Fail || !strings.Contains(f.Value, "did not answer with TLS") {
		t.Fatalf("handshake = %+v", f)
	}

	silent, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	go func() {
		for {
			conn, err := silent.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()
	r, err = TLS(context.Background(), silent.Addr().String(), TLSOptions{Now: now, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "handshake"); f.Status != Fail || !strings.Contains(f.Value, "no TLS answer within 200ms") {
		t.Fatalf("handshake = %+v", f)
	}

	if _, err := TLS(context.Background(), "not a host", TLSOptions{}); err == nil {
		t.Fatal("bad target gave no error")
	}
}
