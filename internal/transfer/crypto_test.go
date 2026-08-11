package transfer

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// testPayload carries a model with nested env/rest fields and an MCP entry
// with headers, matching what real exports contain.
func testPayload() *Payload {
	return &Payload{
		Models: map[string]json.RawMessage{
			"anthropic": json.RawMessage(`{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-ant-test","ANTHROPIC_BASE_URL":"https://api.anthropic.com","ANTHROPIC_MODEL":"claude-sonnet-4-6","HTTP_PROXY":"http://proxy.local:8080"},"model":"sonnet","enabledPlugins":{"git":true}}`),
		},
		MCPs: map[string]json.RawMessage{
			"github": json.RawMessage(`{"type":"http","url":"https://api.githubcopilot.com/mcp/","headers":{"Authorization":"Bearer ghp_secret","X-Api-Key":"abc123"}}`),
		},
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	want := testPayload()
	env, err := Seal(want, "correct horse battery staple")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if env.App != AppID || env.Kind != KindExport || env.Format != FormatVersion {
		t.Fatalf("unexpected envelope identity: %+v", env)
	}
	if env.Cipher != CipherAESGCM || env.KDF != KDFScrypt {
		t.Fatalf("unexpected algorithms: %+v", env)
	}
	if env.KDFParams != (KDFParams{N: 32768, R: 8, P: 1}) {
		t.Fatalf("unexpected kdf params: %+v", env.KDFParams)
	}
	if _, err := time.Parse(time.RFC3339, env.Created); err != nil {
		t.Fatalf("created %q is not RFC3339: %v", env.Created, err)
	}
	if !reflect.DeepEqual(env.Manifest.Models, []string{"anthropic"}) ||
		!reflect.DeepEqual(env.Manifest.MCPs, []string{"github"}) {
		t.Fatalf("unexpected manifest: %+v", env.Manifest)
	}

	got, err := Open(env, "correct horse battery staple")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("payload mismatch after round-trip:\nwant %+v\ngot  %+v", want, got)
	}
}

func TestOpenWrongSecret(t *testing.T) {
	env, err := Seal(testPayload(), "correct-secret-1")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	_, err = Open(env, "wrong-secret-1")
	if !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("want ErrWrongSecret, got %v", err)
	}
}

func TestOpenTamperedManifest(t *testing.T) {
	env, err := Seal(testPayload(), "secret-value-123")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	env.Manifest.Models = []string{"evil"}
	_, err = Open(env, "secret-value-123")
	if !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("want ErrWrongSecret after manifest tampering, got %v", err)
	}
}

func TestOpenTamperedCiphertext(t *testing.T) {
	env, err := Seal(testPayload(), "secret-value-123")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	raw[0] ^= 0xFF
	env.Payload = base64.StdEncoding.EncodeToString(raw)
	_, err = Open(env, "secret-value-123")
	if !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("want ErrWrongSecret after ciphertext tampering, got %v", err)
	}
}

func TestSealOpenCost(t *testing.T) {
	start := time.Now()
	env, err := Seal(testPayload(), "secret-value-123")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := Open(env, "secret-value-123"); err != nil {
		t.Fatalf("open: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("seal+open took %s; scrypt params too costly", elapsed)
	}
}

func TestEnvelopeJSONShape(t *testing.T) {
	env, err := Seal(testPayload(), "secret-value-123")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{
		"app", "kind", "format", "cipher", "kdf", "kdf_params",
		"salt", "nonce", "payload", "manifest", "created", "version",
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("envelope JSON missing key %q", key)
		}
	}
	var kdf map[string]json.RawMessage
	if err := json.Unmarshal(m["kdf_params"], &kdf); err != nil {
		t.Fatalf("kdf_params not an object: %v", err)
	}
	for _, key := range []string{"n", "r", "p"} {
		if _, ok := kdf[key]; !ok {
			t.Errorf("kdf_params missing key %q", key)
		}
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(m["manifest"], &manifest); err != nil {
		t.Fatalf("manifest not an object: %v", err)
	}
	for _, key := range []string{"models", "mcps"} {
		if _, ok := manifest[key]; !ok {
			t.Errorf("manifest missing key %q", key)
		}
	}
}

func TestCheckEnvelope(t *testing.T) {
	env, err := Seal(testPayload(), "secret-value-123")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	validData, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	mutated := func(change func(*Envelope)) []byte {
		t.Helper()
		cp := *env
		cp.Manifest = Manifest{
			Models: append([]string(nil), env.Manifest.Models...),
			MCPs:   append([]string(nil), env.Manifest.MCPs...),
		}
		change(&cp)
		data, err := json.Marshal(&cp)
		if err != nil {
			t.Fatalf("marshal mutated envelope: %v", err)
		}
		return data
	}

	cases := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"valid", validData, ""},
		{"foreign app", mutated(func(e *Envelope) { e.App = "other-tool" }), "not a ccsc export file"},
		{"foreign kind", mutated(func(e *Envelope) { e.Kind = "other-kind" }), "not a ccsc export file"},
		{"future format", mutated(func(e *Envelope) { e.Format = 99 }), "newer ccsc"},
		{"zero format", mutated(func(e *Envelope) { e.Format = 0 }), "unsupported format"},
		{"unknown cipher", mutated(func(e *Envelope) { e.Cipher = "ChaCha20-Poly1305" }), "unsupported cipher"},
		{"unknown kdf", mutated(func(e *Envelope) { e.KDF = "argon2id" }), "unsupported kdf"},
		{"absurd kdf n", mutated(func(e *Envelope) { e.KDFParams.N = 1 << 25 }), "scrypt"},
		{"kdf r too large", mutated(func(e *Envelope) { e.KDFParams.R = 64 }), "scrypt"},
		{"bad base64 salt", mutated(func(e *Envelope) { e.Salt = "!!!" }), "base64"},
		{"short nonce", mutated(func(e *Envelope) { e.Nonce = base64.StdEncoding.EncodeToString([]byte("short")) }), "nonce"},
		{"short payload", mutated(func(e *Envelope) { e.Payload = base64.StdEncoding.EncodeToString([]byte("tiny")) }), "payload"},
		{"garbage", []byte("this is not json"), "not a ccsc export file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckEnvelope(tc.data)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid envelope rejected: %v", err)
				}
				if got.App != AppID || got.Kind != KindExport || got.Format != FormatVersion {
					t.Fatalf("unexpected envelope fields: %+v", got)
				}
				if !reflect.DeepEqual(got.Manifest, env.Manifest) {
					t.Fatalf("manifest changed by check: %+v", got.Manifest)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %q", tc.wantErr, err)
			}
		})
	}
}
