package oidc

import (
	"fmt"
	"regexp"
)

// SSRFValidator validates instance URLs against an allowlist regex.
type SSRFValidator struct {
	fieldPath string
	regex     *regexp.Regexp
}

// NewSSRFValidator creates a new SSRF validator from a regex pattern.
func NewSSRFValidator(fieldPath, pattern string) (*SSRFValidator, error) {
	r, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compiling SSRF allowlist regex: %w", err)
	}
	return &SSRFValidator{fieldPath: fieldPath, regex: r}, nil
}

// Validate checks if the given URL matches the allowlist.
func (v *SSRFValidator) Validate(instanceURL string) error {
	if !v.regex.MatchString(instanceURL) {
		return fmt.Errorf("instance URL %q does not match allowlist", instanceURL)
	}
	return nil
}

// FieldPath returns the field path to extract from userinfo.
func (v *SSRFValidator) FieldPath() string {
	return v.fieldPath
}
