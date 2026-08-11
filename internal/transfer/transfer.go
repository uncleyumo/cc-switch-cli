// Package transfer implements encrypted export and import of model configs
// and user-scope MCP servers. An export is a JSON envelope whose cleartext
// header is authenticated together with the scrypt + AES-256-GCM encrypted
// payload, so both a wrong secret and header tampering are detected on import.
package transfer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// FormatVersion is the envelope format revision this build writes and reads.
	FormatVersion = 1
	// AppID identifies this tool inside export envelopes.
	AppID = "cc-switch-cli"
	// KindExport marks envelopes produced by `ccs export`.
	KindExport = "ccsc-export"
	// CipherAESGCM is the supported payload cipher.
	CipherAESGCM = "AES-256-GCM"
	// KDFScrypt is the supported key derivation function.
	KDFScrypt = "scrypt"
)

// ErrWrongSecret reports a failed payload authentication.
var ErrWrongSecret = errors.New("decryption failed: wrong secret or corrupted file")

// KDFParams holds the scrypt cost parameters stored in the envelope.
type KDFParams struct {
	N int `json:"n"`
	R int `json:"r"`
	P int `json:"p"`
}

// Manifest lists the entry names contained in the payload.
type Manifest struct {
	Models []string `json:"models"`
	MCPs   []string `json:"mcps"`
}

// Envelope is the on-disk export format: cleartext metadata plus an
// authenticated ciphertext payload.
type Envelope struct {
	App       string    `json:"app"`
	Kind      string    `json:"kind"`
	Format    int       `json:"format"`
	Cipher    string    `json:"cipher"`
	KDF       string    `json:"kdf"`
	KDFParams KDFParams `json:"kdf_params"`
	Salt      string    `json:"salt"`    // base64
	Nonce     string    `json:"nonce"`   // base64
	Payload   string    `json:"payload"` // base64 ciphertext incl. GCM tag
	Manifest  Manifest  `json:"manifest"`
	Created   string    `json:"created"` // RFC3339 UTC
	Version   string    `json:"version"` // ccsc version that exported
}

// Payload is the plaintext document. Values are raw JSON bytes so entries
// round-trip without re-encoding.
type Payload struct {
	Models map[string]json.RawMessage `json:"models,omitempty"`
	MCPs   map[string]json.RawMessage `json:"mcps,omitempty"`
}

// CheckEnvelope parses data and validates format compatibility without the
// secret: envelope shape, known app/kind/cipher/kdf, sane scrypt parameters
// and decodable base64 fields.
func CheckEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("not a ccsc export file: %w", err)
	}
	if env.App != AppID || env.Kind != KindExport {
		return nil, errors.New("not a ccsc export file")
	}
	if env.Format > FormatVersion {
		return nil, fmt.Errorf("exported by a newer ccsc (format %d), upgrade to import", env.Format)
	}
	if env.Format < 1 {
		return nil, fmt.Errorf("unsupported format version %d", env.Format)
	}
	if env.Cipher != CipherAESGCM {
		return nil, fmt.Errorf("unsupported cipher %q", env.Cipher)
	}
	if env.KDF != KDFScrypt {
		return nil, fmt.Errorf("unsupported kdf %q", env.KDF)
	}
	if err := checkKDFParams(env.KDFParams); err != nil {
		return nil, err
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("malformed export file: salt is not valid base64: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("malformed export file: nonce is not valid base64: %w", err)
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("malformed export file: payload is not valid base64: %w", err)
	}
	if len(salt) < minSaltSize {
		return nil, fmt.Errorf("malformed export file: salt must be at least %d bytes", minSaltSize)
	}
	if len(nonce) != gcmNonceSize {
		return nil, fmt.Errorf("malformed export file: nonce must be %d bytes", gcmNonceSize)
	}
	if len(payload) < minPayloadSize {
		return nil, fmt.Errorf("malformed export file: payload must be at least %d bytes", minPayloadSize)
	}
	return &env, nil
}

// checkKDFParams bounds the scrypt cost so a hostile envelope cannot exhaust
// memory or CPU during Open.
func checkKDFParams(p KDFParams) error {
	if p.N < 1024 || p.N > 1<<17 || p.N&(p.N-1) != 0 {
		return fmt.Errorf("unsupported scrypt parameter n=%d (power of two between 1024 and %d required)", p.N, 1<<17)
	}
	if p.R < 1 || p.R > 16 {
		return fmt.Errorf("unsupported scrypt parameter r=%d (must be 1-16)", p.R)
	}
	if p.P < 1 || p.P > 8 {
		return fmt.Errorf("unsupported scrypt parameter p=%d (must be 1-8)", p.P)
	}
	if mem := int64(128) * int64(p.N) * int64(p.R); mem > maxScryptMemory {
		return fmt.Errorf("scrypt memory cost exceeds the %d MiB limit", maxScryptMemory>>20)
	}
	return nil
}
