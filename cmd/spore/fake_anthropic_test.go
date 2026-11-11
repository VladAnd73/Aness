package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// fakeAnthropic serves handler as api.anthropic.com and
// platform.claude.com to a child process. The child reaches it through an
// HTTPS_PROXY that tunnels every CONNECT to a local TLS server, whose
// certificate is signed by a throwaway CA trusted via SSL_CERT_FILE. The
// returned env entries wire a child process to it; nothing leaves the host.
func fakeAnthropic(t *testing.T, handler http.Handler) []string {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "spore test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(caDER)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		DNSNames:     []string{"api.anthropic.com", "platform.claude.com"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewUnstartedServer(handler)
	upstream.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}}}
	upstream.StartTLS()
	t.Cleanup(upstream.Close)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
			return
		}
		dst, err := net.Dial("tcp", upstream.Listener.Addr().String())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = dst.Close()
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		go func() { _, _ = io.Copy(dst, conn); _ = dst.Close() }()
		go func() { _, _ = io.Copy(conn, dst); _ = conn.Close() }()
	}))
	t.Cleanup(proxy.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	writeFile(t, caFile, string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})), 0o644)
	return []string{"HTTPS_PROXY=" + proxy.URL, "SSL_CERT_FILE=" + caFile}
}
