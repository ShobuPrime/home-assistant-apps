// Maps to: shanocast (https://xakcop.com/post/shanocast/) — AirReceiver cert
//          + precomputed-signature nonce-replay trick that lets a fake
//          CASTV2 receiver pass Chrome's openscreen device-auth handshake.
//
// Wire shape (subset of cast_channel.proto):
//
//	message AuthChallenge { /* one-way from sender — we don't decode it */ }
//
//	message AuthResponse {
//	    required bytes signature                 = 1; // length-delimited
//	    required bytes client_auth_certificate   = 2; // length-delimited
//	    repeated bytes intermediate_certificates = 3; // length-delimited
//	}
//
//	message DeviceAuthMessage {
//	    optional AuthChallenge challenge = 1;
//	    optional AuthResponse  response  = 2;
//	    optional AuthError     error     = 3;
//	}
//
// Per shanocast: Chrome's openscreen sender has `enforce_nonce_checking=false`,
// so we can respond to any AuthChallenge with a fixed AuthResponse whose
// signature was precomputed against the AirReceiver cert's well-known nonce.
// Other senders vary in strictness; if a particular sender rejects the
// replay, the user has to upgrade the cert/signature pair on disk.
//
// We do **not** ship the AirReceiver cert or the precomputed signature in
// the image. Users supply them under /share/sonuntius/. If either file is
// missing, NewAirReceiverResponder returns a non-nil Responder that
// degrades gracefully: BuildResponse logs at warn and returns an empty
// AuthResponse. The Phase 3b cmd binary checks for the cert path up front
// and refuses to start the TLS server when neither cert nor key exist, so
// in practice an empty BuildResponse never makes it onto the wire — but the
// non-crash path matters for unit tests and for partial provisioning.
package auth

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
)

// Responder is the interface the castv2 server uses to assemble the
// device-auth response. It takes the raw AuthChallenge bytes (currently
// ignored — the shanocast trick is challenge-agnostic) and returns the raw
// DeviceAuthMessage bytes carrying the AuthResponse.
type Responder interface {
	BuildResponse(challenge []byte) ([]byte, error)
}

// AirReceiverResponder is a Responder backed by three user-supplied files
// living next to the cert path:
//
//	<certPath>                    : PEM-encoded AirReceiver X.509 cert
//	<signaturePath>               : raw bytes of the precomputed signature
//	                                over the well-known nonce
//	<intermediatesPath>           : PEM-encoded intermediate certs (concatenated)
//
// The signature and intermediate-cert files are optional. If absent the
// AuthResponse is emitted with those fields empty; Chrome senders with
// loose nonce checking will still accept the response when only the cert
// is present.
type AirReceiverResponder struct {
	certDER       []byte
	signature     []byte
	intermediates [][]byte
	logger        *slog.Logger
}

// NewAirReceiverResponder loads (and validates) the AirReceiver cert and
// any companion signature / intermediate files from disk. If certPath is
// empty or unreadable, the function returns a non-nil responder that
// degrades to an empty AuthResponse with a clear warning logged on every
// BuildResponse call. This lets the cmd binary instantiate the responder
// unconditionally and decide separately whether to actually start the TLS
// server.
//
// Companion file paths follow the convention documented in the package
// comment:
//   - signaturePath / intermediatesPath of "" disable that field
//   - missing files are treated as empty, with a debug-level log
//   - malformed PEM is a hard error (we want loud failure on bad input,
//     since silently producing a partial AuthResponse is worse than
//     crashing the server's startup)
func NewAirReceiverResponder(certPath, signaturePath, intermediatesPath string, logger *slog.Logger) (*AirReceiverResponder, error) {
	if logger == nil {
		logger = slog.Default()
	}
	r := &AirReceiverResponder{logger: logger}

	if certPath == "" {
		logger.Warn("auth: AirReceiver cert path not configured — AuthResponse will be empty")
		return r, nil
	}
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Warn("auth: AirReceiver cert missing — AuthResponse will be empty",
				"path", certPath)
			return r, nil
		}
		return nil, fmt.Errorf("auth: read cert %q: %w", certPath, err)
	}
	der, err := decodeCertPEM(certPEM)
	if err != nil {
		return nil, fmt.Errorf("auth: parse cert %q: %w", certPath, err)
	}
	r.certDER = der

	if signaturePath != "" {
		sig, err := os.ReadFile(signaturePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				logger.Debug("auth: signature file missing — AuthResponse.signature will be empty",
					"path", signaturePath)
			} else {
				return nil, fmt.Errorf("auth: read signature %q: %w", signaturePath, err)
			}
		} else {
			r.signature = sig
		}
	}

	if intermediatesPath != "" {
		bundle, err := os.ReadFile(intermediatesPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				logger.Debug("auth: intermediates file missing", "path", intermediatesPath)
			} else {
				return nil, fmt.Errorf("auth: read intermediates %q: %w", intermediatesPath, err)
			}
		} else {
			r.intermediates, err = decodeIntermediatesPEM(bundle)
			if err != nil {
				return nil, fmt.Errorf("auth: parse intermediates %q: %w", intermediatesPath, err)
			}
		}
	}

	logger.Info("auth: AirReceiver responder ready",
		"cert_bytes", len(r.certDER),
		"signature_bytes", len(r.signature),
		"intermediates", len(r.intermediates))
	return r, nil
}

// BuildResponse returns the wire bytes for a DeviceAuthMessage whose
// `response` field is our AuthResponse. The challenge is ignored — the
// shanocast trick relies on the precomputed signature working against
// senders that skip nonce verification.
//
// When the responder was constructed without a cert (graceful-degrade path)
// the returned bytes encode a DeviceAuthMessage with an empty AuthResponse
// — callers should not actually transmit this; the Phase 3b cmd binary
// will refuse to start the TLS server in that case.
func (r *AirReceiverResponder) BuildResponse(challenge []byte) ([]byte, error) {
	if r.certDER == nil {
		r.logger.Warn("auth: BuildResponse called with no cert loaded — emitting empty AuthResponse")
	}
	authResp := encodeAuthResponse(r.signature, r.certDER, r.intermediates)
	return encodeDeviceAuthMessage(authResp), nil
}

// CertLoaded reports whether the responder actually has an AirReceiver
// cert in memory. Useful for the cmd binary's startup-time gate.
func (r *AirReceiverResponder) CertLoaded() bool {
	return len(r.certDER) > 0
}

// ----- protobuf helpers (hand-rolled, length-delimited fields only) -----

// encodeAuthResponse builds the wire form of:
//
//	message AuthResponse {
//	    required bytes signature                 = 1;
//	    required bytes client_auth_certificate   = 2;
//	    repeated bytes intermediate_certificates = 3;
//	}
//
// Fields with empty values are still emitted because the upstream proto
// marks tags 1 and 2 as `required` — omitting them would cause strict
// senders to reject the response with a parse error.
func encodeAuthResponse(signature, cert []byte, intermediates [][]byte) []byte {
	out := make([]byte, 0, 16+len(signature)+len(cert))
	out = appendBytesField(out, 1, signature)
	out = appendBytesField(out, 2, cert)
	for _, im := range intermediates {
		out = appendBytesField(out, 3, im)
	}
	return out
}

// encodeDeviceAuthMessage wraps an AuthResponse in the DeviceAuthMessage
// envelope at tag 2. (Tag 1 is the inbound AuthChallenge — we never emit
// that; tag 3 is AuthError, used to signal a refusal — we never emit that
// either in the shanocast happy path.)
func encodeDeviceAuthMessage(authResponse []byte) []byte {
	out := make([]byte, 0, 4+len(authResponse))
	out = appendBytesField(out, 2, authResponse)
	return out
}

// appendBytesField emits (tag<<3|2)-prefixed length-delimited bytes.
// Mirrors the helper in ../castmessage.go; duplicated here so the auth
// package has zero dependency on the parent package.
func appendBytesField(out []byte, fieldNum int, value []byte) []byte {
	key := uint64(fieldNum)<<3 | 2
	out = appendVarint(out, key)
	out = appendVarint(out, uint64(len(value)))
	out = append(out, value...)
	return out
}

// appendVarint base-128-encodes v.
func appendVarint(out []byte, v uint64) []byte {
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	return append(out, byte(v))
}

// ----- PEM helpers -----

// decodeCertPEM parses the first CERTIFICATE block from pemBytes and
// returns its DER bytes. We validate that crypto/x509 can parse the cert
// up front so that bad inputs surface as startup errors rather than
// runtime AuthResponse oddities.
func decodeCertPEM(pemBytes []byte) ([]byte, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("unexpected PEM type %q", block.Type)
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return nil, fmt.Errorf("x509 parse: %w", err)
	}
	return block.Bytes, nil
}

// decodeIntermediatesPEM splits a concatenated PEM bundle into the DER
// bytes of each CERTIFICATE block in order. Non-CERTIFICATE blocks are
// rejected so a misplaced private key in the bundle is loud, not silent.
func decodeIntermediatesPEM(pemBytes []byte) ([][]byte, error) {
	var out [][]byte
	rest := pemBytes
	for {
		block, remainder := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			return nil, fmt.Errorf("unexpected PEM type %q in intermediates", block.Type)
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, fmt.Errorf("x509 parse intermediate: %w", err)
		}
		out = append(out, block.Bytes)
		rest = remainder
	}
	return out, nil
}
