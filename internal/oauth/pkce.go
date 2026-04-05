package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// VerifyPKCE verifies the PKCE code_verifier against the stored code_challenge.
func VerifyPKCE(codeVerifier, codeChallenge string) error {
	h := sha256.Sum256([]byte(codeVerifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])

	if computed != codeChallenge {
		return fmt.Errorf("PKCE verification failed")
	}
	return nil
}
