package security

import (
	"strings"
	"testing"
)

func TestGenerateBridgeSecretReturnsDisplayPrefixAndHighEntropySecret(t *testing.T) {
	raw, prefix, err := GenerateBridgeSecret()
	if err != nil {
		t.Fatalf("GenerateBridgeSecret: %v", err)
	}
	if !strings.HasPrefix(raw, "theia_bridge_") {
		t.Fatalf("raw secret prefix = %q, want theia_bridge_...", raw)
	}
	if !strings.HasPrefix(prefix, "theia_bridge_") {
		t.Fatalf("display prefix = %q, want theia_bridge_...", prefix)
	}
	if !strings.HasPrefix(raw, prefix+".") {
		t.Fatalf("raw secret %q does not include display prefix %q", raw, prefix)
	}
	if len(raw) < len("theia_bridge_")+16+1+43 {
		t.Fatalf("raw secret length = %d, want at least recognizable prefix + 32 bytes entropy", len(raw))
	}

	otherRaw, otherPrefix, err := GenerateBridgeSecret()
	if err != nil {
		t.Fatalf("GenerateBridgeSecret second call: %v", err)
	}
	if raw == otherRaw {
		t.Fatal("GenerateBridgeSecret returned duplicate raw secrets")
	}
	if prefix == otherPrefix {
		t.Fatal("GenerateBridgeSecret returned duplicate display prefixes")
	}
}

func TestBridgeSecretHashAndVerifyDoNotStorePlaintext(t *testing.T) {
	raw, _, err := GenerateBridgeSecret()
	if err != nil {
		t.Fatalf("GenerateBridgeSecret: %v", err)
	}

	hash, err := HashBridgeSecret(raw)
	if err != nil {
		t.Fatalf("HashBridgeSecret: %v", err)
	}
	if hash == "" || strings.Contains(hash, raw) {
		t.Fatalf("hash %q exposes raw secret", hash)
	}
	if !VerifyBridgeSecret(raw, hash) {
		t.Fatal("VerifyBridgeSecret rejected matching raw secret")
	}
	if VerifyBridgeSecret(raw+"x", hash) {
		t.Fatal("VerifyBridgeSecret accepted modified raw secret")
	}
	if VerifyBridgeSecret(raw, "sha256:not-valid-base64") {
		t.Fatal("VerifyBridgeSecret accepted malformed hash")
	}
}

func TestBridgeSecretPrefixRejectsMalformedSecrets(t *testing.T) {
	raw, prefix, err := GenerateBridgeSecret()
	if err != nil {
		t.Fatalf("GenerateBridgeSecret: %v", err)
	}
	got, err := BridgeSecretPrefix(raw)
	if err != nil {
		t.Fatalf("BridgeSecretPrefix: %v", err)
	}
	if got != prefix {
		t.Fatalf("BridgeSecretPrefix = %q, want %q", got, prefix)
	}

	for _, value := range []string{"", "theia_bridge_missingdot", "other_1234567890abcdef.token", "theia_bridge_short.x"} {
		if got, err := BridgeSecretPrefix(value); err == nil {
			t.Fatalf("BridgeSecretPrefix(%q) = %q, want error", value, got)
		}
	}
}
