//go:build e2e

package imagebuilder

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrBuildxMissing is returned when `docker buildx` is not available.
var ErrBuildxMissing = errors.New("docker buildx not available")

// BuildxAvailable reports whether `docker buildx version` exits cleanly.
func BuildxAvailable() bool {
	return exec.Command("docker", "buildx", "version").Run() == nil
}

// isBuildah returns true when the buildx backend is buildah/podman rather than
// Docker BuildKit. Buildah lacks --push, --attest, and type=registry output.
func isBuildah() bool {
	out, _ := exec.Command("docker", "buildx", "version").Output()
	return strings.Contains(strings.ToLower(string(out)), "buildah")
}

// isPodmanCLI reports whether the `docker` command is actually podman's shim.
// Docker's CLI rejects the `--tls-verify` flag that podman accepts, so flag
// decisions branch on this.
func isPodmanCLI() bool {
	out, _ := exec.Command("docker", "--version").Output()
	return strings.Contains(strings.ToLower(string(out)), "podman")
}

// BuildxOptions describe a buildx shellout invocation.
type BuildxOptions struct {
	ContextDir string
	Dockerfile string // name of Dockerfile inside context
	Ref        string
	Platforms  string // e.g. "linux/amd64,linux/arm64"
	Attest     string // e.g. "type=provenance,mode=max"
	Insecure   bool
	Creds      Creds
}

// BuildxPush invokes docker buildx to build and push ref with the given options.
// When buildx is absent the function returns ErrBuildxMissing so scenarios can skip.
// On buildah/podman (which lacks --attest and type=registry output), multi-arch builds
// use a podman-manifest fallback; attest requests skip via ErrBuildxMissing.
func BuildxPush(opts BuildxOptions) ([]byte, []byte, error) {
	if !BuildxAvailable() {
		return nil, nil, ErrBuildxMissing
	}
	useDockerAuthConfig := opts.Insecure && !isPodmanCLI()
	if !useDockerAuthConfig {
		if err := dockerLogin(opts.Ref, opts.Creds, opts.Insecure); err != nil {
			return nil, nil, err
		}
	}
	var env []string
	if useDockerAuthConfig {
		cfgDir, err := dockerAuthConfig(opts.Ref, opts.Creds)
		if err != nil {
			return nil, nil, err
		}
		defer os.RemoveAll(cfgDir)
		env = append(os.Environ(), "DOCKER_CONFIG="+cfgDir)
	} else {
		env = append(os.Environ())
	}
	if isBuildah() {
		if opts.Attest != "" {
			// buildah does not implement --attest; skip so the scenario is skipped.
			return nil, nil, ErrBuildxMissing
		}
		return buildahManifestPush(opts)
	}
	args := []string{"buildx", "build"}
	if opts.Insecure && isPodmanCLI() {
		args = append(args, "--tls-verify=false")
	}
	if opts.Dockerfile != "" {
		args = append(args, "-f", opts.Dockerfile)
	}
	if opts.Platforms != "" {
		args = append(args, "--platform", opts.Platforms)
	}
	if opts.Attest != "" {
		args = append(args, "--attest", opts.Attest)
	}
	if opts.Insecure && !isPodmanCLI() {
		args = append(args, "--output", fmt.Sprintf("type=registry,name=%s,push=true,registry.insecure=true", opts.Ref))
	} else {
		args = append(args, "--push", "-t", opts.Ref)
	}
	args = append(args, opts.ContextDir)
	cmd := exec.Command("docker", args...)
	noDefaultAttestations := "1"
	if opts.Attest != "" {
		noDefaultAttestations = "0"
	}
	cmd.Env = append(env, "BUILDX_NO_DEFAULT_ATTESTATIONS="+noDefaultAttestations)
	out, err := cmd.CombinedOutput()
	return out, nil, err
}

func dockerAuthConfig(ref string, creds Creds) (string, error) {
	if creds.Username == "" {
		return "", nil
	}
	host := strings.SplitN(ref, "/", 2)[0]
	dir, err := os.MkdirTemp("", "e2e-docker-config-")
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(dir, "config.json")
	payload := map[string]any{
		"auths": map[string]map[string]string{
			host: {
				"auth": base64.StdEncoding.EncodeToString([]byte(creds.Username + ":" + creds.Password)),
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// buildahManifestPush builds each platform image separately and assembles them
// into a manifest list using `podman manifest`, which is available as `docker manifest`.
func buildahManifestPush(opts BuildxOptions) ([]byte, []byte, error) {
	platforms := strings.Split(opts.Platforms, ",")
	var combined []byte
	localManifest := "e2e-manifest-" + strings.NewReplacer("/", "-", ":", "-", ".", "-").Replace(opts.Ref)
	var perPlatformTags []string

	for _, platform := range platforms {
		suffix := strings.ReplaceAll(strings.TrimSpace(platform), "/", "-")
		remoteTag := opts.Ref + "-" + suffix
		localTag := localManifest + "-" + suffix
		perPlatformTags = append(perPlatformTags, localTag)

		args := []string{"build", "--platform", strings.TrimSpace(platform)}
		if opts.Insecure {
			args = append(args, "--tls-verify=false")
		}
		if opts.Dockerfile != "" {
			args = append(args, "-f", opts.Dockerfile)
		}
		args = append(args, "-t", localTag, opts.ContextDir)
		out, err := exec.Command("docker", args...).CombinedOutput()
		combined = append(combined, out...)
		if err != nil {
			return combined, nil, fmt.Errorf("build %s: %v: %s", platform, err, out)
		}

		tagOut, err := exec.Command("docker", "tag", localTag, remoteTag).CombinedOutput()
		combined = append(combined, tagOut...)
		if err != nil {
			return combined, nil, fmt.Errorf("tag %s as %s: %v: %s", localTag, remoteTag, err, tagOut)
		}

		pushArgs := []string{"push"}
		if opts.Insecure && isPodmanCLI() {
			pushArgs = append(pushArgs, "--tls-verify=false")
		}
		pushArgs = append(pushArgs, remoteTag)
		out, err = exec.Command("docker", pushArgs...).CombinedOutput()
		combined = append(combined, out...)
		if err != nil {
			return combined, nil, fmt.Errorf("push %s: %v: %s", remoteTag, err, out)
		}
	}

	// Remove any pre-existing local manifest list before creating a fresh one.
	exec.Command("docker", "manifest", "rm", localManifest).Run() //nolint:errcheck

	manifestArgs := []string{"manifest", "create", localManifest}
	out, err := exec.Command("docker", manifestArgs...).CombinedOutput()
	combined = append(combined, out...)
	if err != nil {
		return combined, nil, fmt.Errorf("manifest create: %v: %s", err, out)
	}

	for _, tag := range perPlatformTags {
		addArgs := []string{"manifest", "add"}
		if opts.Insecure && isPodmanCLI() {
			addArgs = append(addArgs, "--tls-verify=false")
		}
		addArgs = append(addArgs, localManifest, tag)
		out, err = exec.Command("docker", addArgs...).CombinedOutput()
		combined = append(combined, out...)
		if err != nil {
			return combined, nil, fmt.Errorf("manifest add %s: %v: %s", tag, err, out)
		}
	}

	pushArgs := []string{"manifest", "push"}
	if opts.Insecure && isPodmanCLI() {
		pushArgs = append(pushArgs, "--tls-verify=false")
	}
	pushArgs = append(pushArgs, "--all", localManifest, "docker://"+opts.Ref)
	out, err = exec.Command("docker", pushArgs...).CombinedOutput()
	combined = append(combined, out...)
	if err != nil {
		return combined, nil, fmt.Errorf("manifest push: %v: %s", err, out)
	}

	return combined, nil, nil
}

// dockerLogin issues `docker login` for the registry portion of ref using creds.
// When insecure is true, passes --tls-verify=false (supported by podman and compatible CLIs).
func dockerLogin(ref string, creds Creds, insecure bool) error {
	if creds.Username == "" {
		return nil
	}
	host := strings.SplitN(ref, "/", 2)[0]
	loginTarget := host
	if insecure && !isPodmanCLI() {
		loginTarget = "http://" + host
	}
	args := []string{"login", loginTarget, "-u", creds.Username, "--password-stdin"}
	if insecure && isPodmanCLI() {
		args = append(args, "--tls-verify=false")
	}
	cmd := exec.Command("docker", args...)
	cmd.Stdin = strings.NewReader(creds.Password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker login %s: %v: %s", host, err, out)
	}
	return nil
}
