// Maps to: N/A — Go-only tests for the AirReceiver auth responder.
//
// We verify three things:
//
//  1. NewAirReceiverResponder gracefully handles a missing cert path
//     (returns a responder that emits an empty AuthResponse and logs).
//  2. With a valid cert + signature on disk, the emitted DeviceAuthMessage
//     bytes embed the expected `client_auth_certificate` and `signature`
//     fields.
//  3. Malformed PEM is a hard error at construction.
//
// We generate a fresh self-signed cert in the test so we don't ship any
// real AirReceiver artifact in the repo.
package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewAirReceiverResponderMissingCert(t *testing.T) {
	r, err := NewAirReceiverResponder("", "", "", testLogger())
	if err != nil {
		t.Fatalf("NewAirReceiverResponder empty path: %v", err)
	}
	if r.CertLoaded() {
		t.Error("CertLoaded() = true with no cert provided")
	}
	out, err := r.BuildResponse(nil)
	if err != nil {
		t.Fatalf("BuildResponse: %v", err)
	}
	// Empty cert path -> DeviceAuthMessage wraps an AuthResponse with two
	// empty length-delimited fields (signature, client_auth_certificate).
	// The minimum encoding is: tag 2 length-delim, len=4, then bytes
	// {0x0a 0x00 0x12 0x00}.
	want := []byte{0x12, 0x04, 0x0a, 0x00, 0x12, 0x00}
	if string(out) != string(want) {
		t.Errorf("BuildResponse with empty cert = % x want % x", out, want)
	}
}

func TestNewAirReceiverResponderNonexistentCert(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.pem")
	r, err := NewAirReceiverResponder(missing, "", "", testLogger())
	if err != nil {
		t.Fatalf("NewAirReceiverResponder missing file: %v", err)
	}
	if r.CertLoaded() {
		t.Error("CertLoaded() = true with missing cert file")
	}
}

func TestNewAirReceiverResponderMalformedCert(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.pem")
	if err := os.WriteFile(p, []byte("not pem"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAirReceiverResponder(p, "", "", testLogger()); err == nil {
		t.Fatal("expected error on malformed PEM, got nil")
	}
}

func TestBuildResponseEncodesCertAndSignature(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	sigPath := filepath.Join(dir, "cert.signature")

	certDER := generateSelfSignedCert(t)
	writePEM(t, certPath, "CERTIFICATE", certDER)

	signature := []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(sigPath, signature, 0600); err != nil {
		t.Fatal(err)
	}

	r, err := NewAirReceiverResponder(certPath, sigPath, "", testLogger())
	if err != nil {
		t.Fatalf("NewAirReceiverResponder: %v", err)
	}
	if !r.CertLoaded() {
		t.Fatal("CertLoaded() = false after loading cert")
	}

	out, err := r.BuildResponse(nil)
	if err != nil {
		t.Fatalf("BuildResponse: %v", err)
	}

	// Decode: DeviceAuthMessage(tag 2) -> AuthResponse with
	// signature(tag 1) + client_auth_certificate(tag 2).
	sig, cert := decodeDeviceAuthMessage(t, out)
	if string(sig) != string(signature) {
		t.Errorf("signature = % x want % x", sig, signature)
	}
	if string(cert) != string(certDER) {
		t.Errorf("cert mismatch len=%d want=%d", len(cert), len(certDER))
	}
}

func TestBuildResponseWithIntermediates(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	intermPath := filepath.Join(dir, "intermediates.pem")

	rootDER := generateSelfSignedCert(t)
	intermDER := generateSelfSignedCert(t)
	writePEM(t, certPath, "CERTIFICATE", rootDER)

	// Two-PEM bundle.
	f, err := os.Create(intermPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: intermDER}); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	r, err := NewAirReceiverResponder(certPath, "", intermPath, testLogger())
	if err != nil {
		t.Fatalf("NewAirReceiverResponder: %v", err)
	}
	out, err := r.BuildResponse(nil)
	if err != nil {
		t.Fatalf("BuildResponse: %v", err)
	}

	// We expect the inner AuthResponse to contain one intermediate at tag 3.
	authResp := unwrapDeviceAuthMessage(t, out)
	intermediates := decodeIntermediateRecords(authResp)
	if len(intermediates) != 1 {
		t.Fatalf("intermediates count = %d want 1", len(intermediates))
	}
	if string(intermediates[0]) != string(intermDER) {
		t.Errorf("intermediate cert bytes mismatch")
	}
}

// ---------- test helpers ----------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func generateSelfSignedCert(t *testing.T) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "sonuntius-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		t.Fatal(err)
	}
}

// decodeDeviceAuthMessage walks the hand-rolled wire bytes to extract the
// signature and client_auth_certificate fields. Defensive but minimal —
// we want a test-side decoder that does not share code with the encoder.
func decodeDeviceAuthMessage(t *testing.T, b []byte) (signature, cert []byte) {
	t.Helper()
	authResp := unwrapDeviceAuthMessage(t, b)
	sig, cert, _ := decodeAuthResponse(authResp)
	return sig, cert
}

func unwrapDeviceAuthMessage(t *testing.T, b []byte) []byte {
	t.Helper()
	// Expect tag 2 wire 2 (key = 18).
	if len(b) < 2 || b[0] != 0x12 {
		t.Fatalf("DeviceAuthMessage: bad tag 0x%x", b[0])
	}
	v, n, err := readTestVarint(b[1:])
	if err != nil {
		t.Fatalf("DeviceAuthMessage length varint: %v", err)
	}
	start := 1 + n
	end := start + int(v)
	if end > len(b) {
		t.Fatalf("DeviceAuthMessage truncated len=%d want=%d", len(b), end)
	}
	return b[start:end]
}

func decodeAuthResponse(b []byte) (sig, cert []byte, intermediates [][]byte) {
	i := 0
	for i < len(b) {
		key := b[i]
		i++
		v, n, err := readTestVarint(b[i:])
		if err != nil {
			return
		}
		i += n
		fieldEnd := i + int(v)
		switch key {
		case 0x0a: // tag 1 wire 2
			sig = b[i:fieldEnd]
		case 0x12: // tag 2 wire 2
			cert = b[i:fieldEnd]
		case 0x1a: // tag 3 wire 2
			intermediates = append(intermediates, append([]byte(nil), b[i:fieldEnd]...))
		}
		i = fieldEnd
	}
	return
}

func decodeIntermediateRecords(b []byte) [][]byte {
	_, _, im := decodeAuthResponse(b)
	return im
}

func readTestVarint(b []byte) (uint64, int, error) {
	var v uint64
	var shift uint
	for i, by := range b {
		v |= uint64(by&0x7F) << shift
		if by&0x80 == 0 {
			return v, i + 1, nil
		}
		shift += 7
	}
	return 0, 0, io.ErrUnexpectedEOF
}
