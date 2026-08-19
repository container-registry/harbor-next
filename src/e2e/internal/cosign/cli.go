//go:build e2e

// Package cosign is a thin shellout wrapper around the cosign CLI. Scenarios
// tagged @sigstore skip cleanly when cosign is not installed.
package cosign

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrCosignMissing is returned when the cosign binary is not on $PATH.
var ErrCosignMissing = errors.New("cosign not available")

// Available reports whether the cosign binary is reachable.
func Available() bool {
	return exec.Command("cosign", "version").Run() == nil
}

// KeyPair holds the on-disk paths to a fresh cosign key pair.
type KeyPair struct {
	Dir     string
	KeyPath string // cosign.key
	PubPath string // cosign.pub
}

// Generate generates a fresh cosign key pair inside dir with an empty password.
func Generate(dir string) (*KeyPair, error) {
	if !Available() {
		return nil, ErrCosignMissing
	}
	cmd := exec.Command("cosign", "generate-key-pair")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "COSIGN_PASSWORD=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("cosign generate-key-pair: %v\n%s", err, out)
	}
	return &KeyPair{
		Dir:     dir,
		KeyPath: filepath.Join(dir, "cosign.key"),
		PubPath: filepath.Join(dir, "cosign.pub"),
	}, nil
}

// Creds supplies registry credentials to cosign via env vars.
type Creds struct {
	Username string
	Password string
}

func (c Creds) env() []string {
	e := append([]string(nil), os.Environ()...)
	if c.Username != "" {
		e = append(e,
			"COSIGN_PASSWORD=",
			"COSIGN_USER="+c.Username,
			"COSIGN_REGISTRY_USERNAME="+c.Username,
			"COSIGN_REGISTRY_PASSWORD="+c.Password,
			"COSIGN_DOCKER_MEDIA_TYPES=1",
		)
	}
	return e
}

// Sign signs ref with the given key; --allow-insecure-registry is always set
// so that dev-stack plaintext HTTP works. Returns combined stdout+stderr.
func Sign(ref, keyPath string, creds Creds) ([]byte, error) {
	if !Available() {
		return nil, ErrCosignMissing
	}
	cmd := exec.Command("cosign", "sign", "--yes",
		"--allow-insecure-registry", "--allow-http-registry",
		"--key", keyPath, ref)
	cmd.Env = creds.env()
	return cmd.CombinedOutput()
}

// Verify returns nil on successful verification, non-nil otherwise. stdout is
// always returned for diagnostics.
func Verify(ref, pubPath string, creds Creds) ([]byte, error) {
	if !Available() {
		return nil, ErrCosignMissing
	}
	cmd := exec.Command("cosign", "verify",
		"--insecure-ignore-tlog=true",
		"--allow-insecure-registry", "--allow-http-registry",
		"--key", pubPath, ref)
	cmd.Env = creds.env()
	return cmd.CombinedOutput()
}

// Attest attaches a predicate file of the given type as an in-toto attestation.
// --new-bundle-format=true forces cosign to publish the attestation through the
// OCI 1.1 referrers API, which Harbor records via the subject middleware.
func Attest(ref, keyPath, predicatePath, predicateType string, creds Creds) ([]byte, error) {
	if !Available() {
		return nil, ErrCosignMissing
	}
	cmd := exec.Command("cosign", "attest", "--yes",
		"--allow-insecure-registry", "--allow-http-registry",
		"--new-bundle-format=true",
		"--type", predicateType,
		"--predicate", predicatePath,
		"--key", keyPath, ref)
	cmd.Env = creds.env()
	return cmd.CombinedOutput()
}

// VerifyAttestation returns nil when a matching attestation can be fetched and
// verified for ref. stdout is returned for diagnostics.
func VerifyAttestation(ref, pubPath, predicateType string, creds Creds) ([]byte, error) {
	if !Available() {
		return nil, ErrCosignMissing
	}
	cmd := exec.Command("cosign", "verify-attestation",
		"--insecure-ignore-tlog=true",
		"--allow-insecure-registry", "--allow-http-registry",
		"--type", predicateType,
		"--key", pubPath, ref)
	cmd.Env = creds.env()
	return cmd.CombinedOutput()
}

// IsMissingSignatureErr reports whether err indicates "no signature found".
func IsMissingSignatureErr(output []byte) bool {
	s := strings.ToLower(string(output))
	return strings.Contains(s, "no matching signatures") || strings.Contains(s, "no signatures found")
}
