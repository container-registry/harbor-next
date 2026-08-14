//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/e2e/internal/harborclient"
	"github.com/goharbor/harbor/src/e2e/internal/imagebuilder"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

// scenarioID returns the scenario's unique suffix (`e2e-<hex>`).
func scenarioID(s *state.Scenario) string { return "e2e-" + s.Suffix }

// projectName derives the actual project name from a scenario-local alias.
// The spec uses names like "alpha" / "proj-a" inside feature files; we append
// the per-scenario suffix so parallel runs never collide.
func projectName(s *state.Scenario, alias string) string {
	return fmt.Sprintf("%s-%s", alias, s.Suffix)
}

// userName derives the actual user name from a scenario-local alias.
func userName(s *state.Scenario, alias string) string {
	return fmt.Sprintf("%s-%s", alias, s.Suffix)
}

// repoWithoutTag strips the ":tag" suffix from a fully qualified image
// reference (host:port/project/repo:tag → host:port/project/repo), without
// accidentally stripping the host port.
func repoWithoutTag(ref string) string {
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		return ref[:i]
	}
	return ref
}

// parseAliasRef splits a feature-file image reference of the form
// "<projectAlias>/<repo>:<tag>" into its parts.
func parseAliasRef(ref string) (projectAlias, repo, tag string, err error) {
	slash := strings.Index(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if slash < 0 || colon < 0 || colon < slash {
		return "", "", "", fmt.Errorf("invalid image ref %q; expected <project>/<repo>:<tag>", ref)
	}
	return ref[:slash], ref[slash+1 : colon], ref[colon+1:], nil
}

// registryRef resolves a feature-file reference into a full push/pull URL by
// substituting the aliased project with its actual suffix-bearing name.
func registryRef(s *state.Scenario, ref string) (string, string /* project */, string /* repo */, string /* tag */, error) {
	alias, repo, tag, err := parseAliasRef(ref)
	if err != nil {
		return "", "", "", "", err
	}
	project := projectName(s, alias)
	full := fmt.Sprintf("%s/%s/%s:%s", s.RegistryHost, project, repo, tag)
	return full, project, repo, tag, nil
}

// adminCreds returns the crane-usable credentials for the admin client.
func adminCreds(s *state.Scenario) imagebuilder.Creds {
	return imagebuilder.Creds{Username: s.Client.Username, Password: s.Client.Password}
}

func userCreds(u state.UserCred) imagebuilder.Creds {
	return imagebuilder.Creds{Username: u.Name, Password: u.Password}
}

// craneOpts returns options for the admin credentials, honouring HTTP vs HTTPS.
func adminCraneOpts(s *state.Scenario) imagebuilder.Options {
	return imagebuilder.Options{Insecure: s.Client.Insecure(), Creds: adminCreds(s)}
}

// userCraneOpts returns options for an aliased user.
func userCraneOpts(s *state.Scenario, alias string) (imagebuilder.Options, error) {
	u, ok := s.UsersByAlias[alias]
	if !ok {
		return imagebuilder.Options{}, fmt.Errorf("no user %q registered in scenario", alias)
	}
	return imagebuilder.Options{Insecure: s.Client.Insecure(), Creds: userCreds(u)}, nil
}

// captureResp stores the last API response on state for subsequent Then assertions.
func captureResp(s *state.Scenario, resp *harborclient.Response, err error) {
	s.LastErr = err
	if resp != nil {
		s.LastResp = &http.Response{StatusCode: resp.StatusCode, Header: resp.Header}
		s.LastBody = resp.Body
	} else {
		s.LastResp = nil
		s.LastBody = nil
	}
}

// storageUnit converts the (amount, unit) tuple from feature files into bytes.
func storageUnit(amount int, unit string) int64 {
	switch unit {
	case "KiB":
		return int64(amount) * 1024
	case "MiB":
		return int64(amount) * 1024 * 1024
	case "GiB":
		return int64(amount) * 1024 * 1024 * 1024
	default:
		return int64(amount)
	}
}

// tempDir allocates a per-scenario temp directory and registers it for cleanup.
func tempDir(s *state.Scenario, pattern string) (string, error) {
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		return "", err
	}
	s.TempDirs = append(s.TempDirs, dir)
	return dir, nil
}

// requireProject panics if the aliased project is not tracked — protects
// feature-file typos from producing confusing runtime errors.
func requireProject(s *state.Scenario, alias string) string {
	name := projectName(s, alias)
	for _, p := range s.CreatedProjects {
		if p == name {
			return name
		}
	}
	return name // tolerate — the step may intentionally reference before-create.
}

// pollUntil polls fn at every interval until it returns (done=true, err=nil)
// or the deadline expires.
func pollUntil(ctx context.Context, interval, timeout time.Duration, fn func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for {
		done, err := fn()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("polling timed out after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// firstJSONMap extracts the first JSON object from an array body, when that's
// what Harbor returned.
func firstJSONMap(body []byte) (map[string]any, error) {
	var arr []map[string]any
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		return arr[0], nil
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err == nil {
		return obj, nil
	}
	return nil, fmt.Errorf("response is neither object nor array")
}

// atoiOr returns i if it parses, or fallback.
func atoiOr(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
