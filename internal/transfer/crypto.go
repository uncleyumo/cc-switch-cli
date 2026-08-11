package transfer

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"golang.org/x/crypto/scrypt"

	"cc-switch-cli/internal/version"
)

const (
	kdfN = 32768
	kdfR = 8
	kdfP = 1

	keySize        = 32
	saltSize       = 16
	minSaltSize    = 8
	gcmNonceSize   = 12
	minPayloadSize = 16

	maxScryptMemory = int64(256) << 20
)

// aadClaims mirrors the cleartext envelope fields in a fixed order; its JSON
// encoding is the GCM additional authenticated data, so any edit to the
// cleartext part of the envelope breaks decryption.
type aadClaims struct {
	App       string    `json:"app"`
	Kind      string    `json:"kind"`
	Format    int       `json:"format"`
	Cipher    string    `json:"cipher"`
	KDF       string    `json:"kdf"`
	KDFParams KDFParams `json:"kdf_params"`
	Salt      string    `json:"salt"`
	Nonce     string    `json:"nonce"`
	Manifest  Manifest  `json:"manifest"`
	Created   string    `json:"created"`
	Version   string    `json:"version"`
}

func envelopeAAD(env *Envelope) ([]byte, error) {
	return json.Marshal(aadClaims{
		App:       env.App,
		Kind:      env.Kind,
		Format:    env.Format,
		Cipher:    env.Cipher,
		KDF:       env.KDF,
		KDFParams: env.KDFParams,
		Salt:      env.Salt,
		Nonce:     env.Nonce,
		Manifest:  env.Manifest,
		Created:   env.Created,
		Version:   env.Version,
	})
}

// Seal encrypts the payload with the secret and returns the export envelope.
func Seal(p *Payload, secret string) (*Envelope, error) {
	if p == nil {
		return nil, errors.New("empty payload")
	}
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	key, err := scrypt.Key([]byte(secret), salt, kdfN, kdfR, kdfP, keySize)
	if err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	plaintext, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload: %w", err)
	}
	env := &Envelope{
		App:       AppID,
		Kind:      KindExport,
		Format:    FormatVersion,
		Cipher:    CipherAESGCM,
		KDF:       KDFScrypt,
		KDFParams: KDFParams{N: kdfN, R: kdfR, P: kdfP},
		Salt:      base64.StdEncoding.EncodeToString(salt),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
		Manifest:  manifestOf(p),
		Created:   time.Now().UTC().Format(time.RFC3339),
		Version:   version.Version,
	}
	aad, err := envelopeAAD(env)
	if err != nil {
		return nil, fmt.Errorf("failed to encode envelope header: %w", err)
	}
	env.Payload = base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plaintext, aad))
	return env, nil
}

// Open authenticates and decrypts the envelope payload with the secret.
// Any authentication failure reports ErrWrongSecret.
func Open(env *Envelope, secret string) (*Payload, error) {
	if env == nil {
		return nil, errors.New("empty envelope")
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, ErrWrongSecret
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, ErrWrongSecret
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, ErrWrongSecret
	}
	key, err := scrypt.Key([]byte(secret), salt, env.KDFParams.N, env.KDFParams.R, env.KDFParams.P, keySize)
	if err != nil {
		return nil, ErrWrongSecret
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrWrongSecret
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrWrongSecret
	}
	aad, err := envelopeAAD(env)
	if err != nil {
		return nil, ErrWrongSecret
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrWrongSecret
	}
	var p Payload
	if err := json.Unmarshal(plaintext, &p); err != nil {
		return nil, fmt.Errorf("failed to parse decrypted payload: %w", err)
	}
	return &p, nil
}

// manifestOf lists the payload entry names, sorted. Empty sides yield empty
// slices so the envelope JSON carries [] instead of null.
func manifestOf(p *Payload) Manifest {
	return Manifest{Models: sortedKeys(p.Models), MCPs: sortedKeys(p.MCPs)}
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
