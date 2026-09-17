package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyPKCES256(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if !verifyPKCE(verifier, challenge, "S256") {
		t.Fatal("expected S256 PKCE to pass")
	}
	if verifyPKCE(verifier, "wrong", "S256") {
		t.Fatal("expected mismatch to fail")
	}
}
