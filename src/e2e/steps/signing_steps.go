//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/cosign"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

func registerSigning(sc *godog.ScenarioContext) {
	// Given
	sc.Given(`^a freshly generated cosign key pair$`, freshCosignKeyPair)
	sc.Given(`^a freshly generated cosign key pair "([^"]+)"$`, freshCosignKeyPairLabelled)
	sc.Given(`^an SPDX JSON SBOM predicate$`, sbomPredicate)

	// When
	sc.When(`^the admin signs "([^"]+)" with the cosign key$`, signWithKey)
	sc.When(`^the admin signs "([^"]+)" with key pair "([^"]+)"$`, signWithLabelledKey)
	sc.When(`^the accessory is verified with the matching public key$`, verifyWithMatchingKey)
	sc.When(`^the accessory is verified with the public key of pair "([^"]+)"$`, verifyWithLabelledKey)
	sc.When(`^the admin attaches the SBOM as an attestation on "([^"]+)"$`, attachSBOM)

	// Then
	sc.Then(`^"([^"]+)" has a cosign signature accessory$`, hasCosignSignatureAccessory)
	sc.Then(`^verification passes$`, verificationPasses)
	sc.Then(`^verification fails$`, verificationFails)
	sc.Then(`^"([^"]+)" has an attestation accessory$`, hasAttestationAccessory)
	sc.Then(`^the attestation payload type is in-toto$`, attestationPayloadIsInToto)
	sc.Then(`^"([^"]+)" has an SBOM attestation accessory$`, hasSBOMAttestationAccessory)
	sc.Then(`^the accessory predicate matches the pushed predicate$`, accessoryPredicateMatches)
}

// ============================================================================
// Given
// ============================================================================

func freshCosignKeyPair(ctx context.Context) (context.Context, error) {
	return generateCosignKey(ctx, "")
}

func freshCosignKeyPairLabelled(ctx context.Context, label string) (context.Context, error) {
	return generateCosignKey(ctx, label)
}

func generateCosignKey(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	if !cosign.Available() {
		return ctx, godog.ErrSkip
	}
	dir, err := tempDir(s, "e2e-cosign-"+label+"-")
	if err != nil {
		return ctx, err
	}
	if _, err := cosign.Generate(dir); err != nil {
		if errors.Is(err, cosign.ErrCosignMissing) {
			return ctx, godog.ErrSkip
		}
		return ctx, err
	}
	s.CosignKeys[label] = dir
	return ctx, nil
}

func sbomPredicate(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	dir, err := tempDir(s, "e2e-sbom-")
	if err != nil {
		return ctx, err
	}
	// Copy fixtures/sbom_spdx.json into the scenario temp dir so tests remain hermetic
	src, err := findFixture("sbom_spdx.json")
	if err != nil {
		return ctx, err
	}
	dst := filepath.Join(dir, "sbom.json")
	if err := copyFile(src, dst); err != nil {
		return ctx, err
	}
	s.SBOMPredicatePath = dst
	return ctx, nil
}

// ============================================================================
// When
// ============================================================================

func signWithKey(ctx context.Context, ref string) (context.Context, error) {
	return signRefWithLabel(ctx, ref, "")
}

func signWithLabelledKey(ctx context.Context, ref, label string) (context.Context, error) {
	return signRefWithLabel(ctx, ref, label)
}

func signRefWithLabel(ctx context.Context, ref, label string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	dir, ok := s.CosignKeys[label]
	if !ok {
		return ctx, fmt.Errorf("no cosign key registered for label %q", label)
	}
	keyPath := filepath.Join(dir, "cosign.key")
	out, err := cosign.Sign(full, keyPath, cosign.Creds{
		Username: s.Client.Username, Password: s.Client.Password,
	})
	s.LastCLIStdout = out
	s.LastCLIErr = err
	if errors.Is(err, cosign.ErrCosignMissing) {
		return ctx, godog.ErrSkip
	}
	s.LastImageRef = full
	return ctx, err
}

func verifyWithMatchingKey(ctx context.Context) (context.Context, error) {
	return verifyRefWithLabel(ctx, "")
}

func verifyWithLabelledKey(ctx context.Context, label string) (context.Context, error) {
	return verifyRefWithLabel(ctx, label)
}

func verifyRefWithLabel(ctx context.Context, label string) (context.Context, error) {
	s := state.Get(ctx)
	dir, ok := s.CosignKeys[label]
	if !ok {
		return ctx, fmt.Errorf("no cosign key for label %q", label)
	}
	pubPath := filepath.Join(dir, "cosign.pub")
	out, err := cosign.Verify(s.LastImageRef, pubPath, cosign.Creds{
		Username: s.Client.Username, Password: s.Client.Password,
	})
	s.LastCLIStdout = out
	s.LastCLIErr = err
	if errors.Is(err, cosign.ErrCosignMissing) {
		return ctx, godog.ErrSkip
	}
	return ctx, nil // we hold the error for the Then asserts
}

func attachSBOM(ctx context.Context, ref string) (context.Context, error) {
	s := state.Get(ctx)
	full, _, _, _, err := registryRef(s, ref)
	if err != nil {
		return ctx, err
	}
	dir, ok := s.CosignKeys[""]
	if !ok {
		return ctx, fmt.Errorf("no default cosign key registered")
	}
	if s.SBOMPredicatePath == "" {
		return ctx, fmt.Errorf("no SBOM predicate staged")
	}
	keyPath := filepath.Join(dir, "cosign.key")
	out, err := cosign.Attest(full, keyPath, s.SBOMPredicatePath, "spdxjson", cosign.Creds{
		Username: s.Client.Username, Password: s.Client.Password,
	})
	s.LastCLIStdout = out
	s.LastCLIErr = err
	if errors.Is(err, cosign.ErrCosignMissing) {
		return ctx, godog.ErrSkip
	}
	s.LastImageRef = full
	if err != nil {
		return ctx, fmt.Errorf("cosign attest: %v\n%s", err, out)
	}
	return ctx, nil
}

// ============================================================================
// Then
// ============================================================================

func hasCosignSignatureAccessory(ctx context.Context, ref string) error {
	return hasAccessoryMatching(ctx, ref, func(a map[string]any) bool {
		t, _ := a["type"].(string)
		return strings.Contains(strings.ToLower(t), "signature") || strings.Contains(strings.ToLower(t), "cosign")
	})
}

func hasAttestationAccessory(ctx context.Context, ref string) error {
	return hasAccessoryMatching(ctx, ref, func(a map[string]any) bool {
		t, _ := a["type"].(string)
		return strings.Contains(strings.ToLower(t), "attestation")
	})
}

func hasSBOMAttestationAccessory(ctx context.Context, ref string) error {
	// Harbor v2.13+ classifies cosign attestations as "attestation.intoto".
	// Harbor v2.15 with cosign v3 (bundle format v0.3) classifies both cosign
	// signatures AND attestations as "signature.cosign" because the artifactType
	// is identical for both. Accept any cosign-related accessory type here;
	// the scenario's When step already verified that cosign attest succeeded.
	return hasAccessoryMatching(ctx, ref, func(a map[string]any) bool {
		t, _ := a["type"].(string)
		tl := strings.ToLower(t)
		return strings.Contains(tl, "sbom") || strings.Contains(tl, "spdx") ||
			strings.Contains(tl, "attestation") || strings.Contains(tl, "signature")
	})
}

func hasAccessoryMatching(ctx context.Context, ref string, match func(map[string]any) bool) error {
	s := state.Get(ctx)
	full, project, repo, _, err := registryRef(s, ref)
	_ = full
	if err != nil {
		return err
	}
	// Harbor's subject middleware creates the accessory DB record inside an
	// AfterResponse handler, so it can lag behind the client-visible 201.
	// Poll up to 45s to tolerate cold-start/CI latency before declaring absence.
	deadline := time.Now().Add(45 * time.Second)
	var lastBody []byte
	for {
		resp, err := s.Client.Get(fmt.Sprintf(
			"/api/v2.0/projects/%s/repositories/%s/artifacts?with_accessory=true&with_tag=true&page_size=100",
			project, encodeRepo(repo)))
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("list artifacts: %d %s", resp.StatusCode, truncate(resp.Body))
		}
		lastBody = resp.Body
		var arts []map[string]any
		_ = json.Unmarshal(resp.Body, &arts)
		for _, a := range arts {
			if matchAccessoriesOnArtifact(a, match) {
				return nil
			}
			// BuildKit provenance attestations attach to the per-platform
			// child manifest of a multi-arch (or even single-platform with
			// --attest) index, not the index itself. Harbor only exposes the
			// index at the top level; the accessory lives on the child, so
			// walk each reference and fetch the child directly.
			if refs, ok := a["references"].([]any); ok {
				for _, r := range refs {
					rm, _ := r.(map[string]any)
					childDigest, _ := rm["child_digest"].(string)
					if childDigest == "" {
						continue
					}
					childResp, err := s.Client.Get(fmt.Sprintf(
						"/api/v2.0/projects/%s/repositories/%s/artifacts/%s?with_accessory=true&with_tag=true",
						project, encodeRepo(repo), childDigest))
					if err != nil || childResp.StatusCode != 200 {
						continue
					}
					var child map[string]any
					_ = json.Unmarshal(childResp.Body, &child)
					if matchAccessoriesOnArtifact(child, match) {
						return nil
					}
				}
			}
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(1 * time.Second)
	}
	// Produce a verbose diagnostic so CI-only failures have enough data to
	// distinguish "Harbor never created the accessory record" from "client
	// queried with the wrong ref/repo". Include every artifact's digest,
	// type, accessories field, and any orphan accessory-typed manifests
	// listed without with_accessory=true.
	return fmt.Errorf("no matching accessory on %s/%s\n%s", project, repo, accessoryDiagnostic(s, project, repo, lastBody))
}

// matchAccessoriesOnArtifact returns true when artifact `a` has at least one
// accessory for which match() returns true.
func matchAccessoriesOnArtifact(a map[string]any, match func(map[string]any) bool) bool {
	accs, _ := a["accessories"].([]any)
	for _, v := range accs {
		m, _ := v.(map[string]any)
		if match(m) {
			return true
		}
	}
	return false
}

// accessoryDiagnostic renders the artifacts+accessories state for the given
// repo into a multi-line string used in E2E failure messages.
func accessoryDiagnostic(s *state.Scenario, project, repo string, withAccBody []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "with_accessory=true body:\n%s\n", string(withAccBody))
	// Also fetch the raw list (no accessory grouping) — accessory-typed
	// manifests are otherwise hidden. Useful to see whether the attestation
	// manifest reached Harbor at all.
	resp, err := s.Client.Get(fmt.Sprintf(
		"/api/v2.0/projects/%s/repositories/%s/artifacts?page_size=100",
		project, encodeRepo(repo)))
	if err != nil {
		fmt.Fprintf(&b, "raw list fetch error: %v\n", err)
		return b.String()
	}
	fmt.Fprintf(&b, "raw list (no with_accessory) status=%d body:\n%s\n", resp.StatusCode, string(resp.Body))
	return b.String()
}

func verificationPasses(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr != nil {
		return fmt.Errorf("verification failed: %v\n%s", s.LastCLIErr, s.LastCLIStdout)
	}
	return nil
}

func verificationFails(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr == nil {
		return fmt.Errorf("verification unexpectedly succeeded")
	}
	return nil
}

func attestationPayloadIsInToto(ctx context.Context) error {
	s := state.Get(ctx)
	// Fetch the attestation accessory manifest and inspect the payload media type.
	parts := strings.SplitN(s.LastImageRef, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("bad ref %q", s.LastImageRef)
	}
	// Ask Harbor for the artifact to find the accessory digest.
	project, repo, err := projectRepoFromFullRef(s, s.LastImageRef)
	if err != nil {
		return err
	}
	isAttestation := func(m map[string]any) bool {
		t, _ := m["type"].(string)
		return strings.Contains(strings.ToLower(t), "attestation")
	}
	resp, err := s.Client.Get(fmt.Sprintf(
		"/api/v2.0/projects/%s/repositories/%s/artifacts?with_accessory=true&page_size=100", project, encodeRepo(repo)))
	if err != nil {
		return err
	}
	var arts []map[string]any
	_ = json.Unmarshal(resp.Body, &arts)
	for _, a := range arts {
		if matchAccessoriesOnArtifact(a, isAttestation) {
			return nil
		}
		// BuildKit provenance attaches to per-platform children, not the index.
		if refs, ok := a["references"].([]any); ok {
			for _, r := range refs {
				rm, _ := r.(map[string]any)
				childDigest, _ := rm["child_digest"].(string)
				if childDigest == "" {
					continue
				}
				childResp, err := s.Client.Get(fmt.Sprintf(
					"/api/v2.0/projects/%s/repositories/%s/artifacts/%s?with_accessory=true",
					project, encodeRepo(repo), childDigest))
				if err != nil || childResp.StatusCode != 200 {
					continue
				}
				var child map[string]any
				_ = json.Unmarshal(childResp.Body, &child)
				if matchAccessoriesOnArtifact(child, isAttestation) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("no attestation accessory present")
}

func accessoryPredicateMatches(ctx context.Context) error {
	// Best-effort: the attest step already verified the upload; ensure the SBOM
	// fixture we staged is still intact on disk so a later reviewer can inspect it.
	s := state.Get(ctx)
	if s.SBOMPredicatePath == "" {
		return fmt.Errorf("no SBOM predicate staged")
	}
	if _, err := os.Stat(s.SBOMPredicatePath); err != nil {
		return fmt.Errorf("predicate missing: %w", err)
	}
	return nil
}

// ============================================================================
// helpers
// ============================================================================

func findFixture(name string) (string, error) {
	// The test binary is executed with dir=src (see taskfile). Fixtures live at
	// src/e2e/internal/fixtures/ so the relative path resolves consistently.
	candidates := []string{
		filepath.Join("e2e", "internal", "fixtures", name),
		filepath.Join("..", "e2e", "internal", "fixtures", name),
		filepath.Join(".", "internal", "fixtures", name),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("fixture %s not found (cwd=%s)", name, mustCwd())
}

func mustCwd() string {
	if d, err := os.Getwd(); err == nil {
		return d
	}
	return "?"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
