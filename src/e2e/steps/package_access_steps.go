//go:build e2e

package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/cucumber/godog"

	"github.com/goharbor/harbor/src/e2e/internal/harborclient"
	"github.com/goharbor/harbor/src/e2e/internal/state"
)

// registerPackageAccess installs the access-control steps for native packages:
// who may read a private project, who may publish into it, and how far a robot
// credential reaches.
//
// The shape of every scenario here is deliberate. A denial on its own is not
// evidence — a feature broken for everybody denies everybody, and the scenario
// would still go green. So each refusal is paired with the same action
// succeeding for someone entitled to it, and those pairs are what these steps
// are built to express.
func registerPackageAccess(sc *godog.ScenarioContext) {
	// Given ---------------------------------------------------------------
	sc.Given(`^a fresh user "([^"]+)" with the "([^"]+)" role on "([^"]+)"$`, aFreshUserWithRoleOn)

	// When ----------------------------------------------------------------
	sc.When(`^"([^"]+)" installs "([^"]+)" from "([^"]+)"$`, userInstalls)
	sc.When(`^"([^"]+)" publishes version "([^"]+)" of "([^"]+)" to "([^"]+)"$`, userPublishesVersion)
	sc.When(`^the robot is disabled$`, theRobotIsDisabled)
	sc.When(`^the robot is deleted$`, theRobotIsDeleted)

	// Then ----------------------------------------------------------------
	sc.Then(`^the install is refused$`, theInstallIsRefused)
	sc.Then(`^the refusal does not reveal whether the package exists$`, theRefusalRevealsNothing)
	sc.Then(`^"([^"]+)" holds only version "([^"]+)" of "([^"]+)"$`, projectHoldsOnlyVersion)
	sc.Then(`^"([^"]+)" can still install "([^"]+)" from "([^"]+)"$`, userCanStillInstall)
}

// ============================================================================
// Arrange
// ============================================================================

// aFreshUserWithRoleOn creates a user and gives it a project role in one step.
// Splitting the two reads as ceremony in every scenario that uses it.
func aFreshUserWithRoleOn(ctx context.Context, alias, role, projectAlias string) (context.Context, error) {
	ctx, err := aFreshUser(ctx, alias)
	if err != nil {
		return ctx, err
	}
	return assignProjectRole(ctx, alias, role, projectAlias)
}

func userCred(s *state.Scenario, alias string) (state.UserCred, error) {
	u, ok := s.UsersByAlias[alias]
	if !ok {
		return state.UserCred{}, fmt.Errorf("no user %q registered in this scenario", alias)
	}
	return u, nil
}

// ============================================================================
// Act
// ============================================================================

func userInstalls(ctx context.Context, alias, base, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	u, err := userCred(s, alias)
	if err != nil {
		return ctx, err
	}
	return npmInstall(ctx, s, base, projectAlias, true, u.Name, u.Password, "")
}

func userPublishesVersion(ctx context.Context, alias, version, base, projectAlias string) (context.Context, error) {
	s := state.Get(ctx)
	if _, err := resolvePkg(s, base); err != nil {
		return ctx, err
	}
	u, err := userCred(s, alias)
	if err != nil {
		return ctx, err
	}
	if err := writeNpmSources(s, version, publishedMarker(s)); err != nil {
		return ctx, err
	}
	return ctx, npmPublishAs(ctx, s, projectAlias, "", u.Name, u.Password)
}

// theRobotIsDisabled switches the robot off without deleting it. Harbor wants
// the whole robot representation back on the PUT, so the current one is read
// first and returned with the flag flipped.
func theRobotIsDisabled(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	rb := latestRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	path := fmt.Sprintf("/api/v2.0/robots/%d", rb.ID)
	resp, err := s.Client.Get(path)
	if err != nil {
		return ctx, err
	}
	if err := harborclient.Expect(resp, 200); err != nil {
		return ctx, fmt.Errorf("read robot %d: %w", rb.ID, err)
	}
	var robot map[string]any
	if err := json.Unmarshal(resp.Body, &robot); err != nil {
		return ctx, fmt.Errorf("decode robot: %w", err)
	}
	robot["disable"] = true
	resp, err = s.Client.Put(path, robot)
	if err != nil {
		return ctx, err
	}
	return ctx, harborclient.Expect(resp, 200)
}

func theRobotIsDeleted(ctx context.Context) (context.Context, error) {
	s := state.Get(ctx)
	rb := latestRobot(s)
	if rb == nil {
		return ctx, fmt.Errorf("no robot created in scenario")
	}
	resp, err := s.Client.Delete(fmt.Sprintf("/api/v2.0/robots/%d", rb.ID))
	if err != nil {
		return ctx, err
	}
	return ctx, harborclient.Expect(resp, 200)
}

// ============================================================================
// Assert
// ============================================================================

func theInstallIsRefused(ctx context.Context) error {
	s := state.Get(ctx)
	if s.LastCLIErr == nil {
		return fmt.Errorf("install succeeded but should have been refused\nstdout:\n%s", s.LastCLIStdout)
	}
	out := string(s.LastCLIStdout) + string(s.LastCLIStderr)
	if !strings.Contains(out, "401") && !strings.Contains(out, "403") && !strings.Contains(out, "404") {
		return fmt.Errorf("install failed without an authorization status from the registry: %s", cliFailure(s))
	}
	return nil
}

// theRefusalRevealsNothing checks the clause that is easy to miss: blocking the
// outsider is only half of it. If a package that exists were refused
// differently from a name that does not, an outsider could map a company's
// private library names one guess at a time without ever getting in.
func theRefusalRevealsNothing(ctx context.Context) error {
	s := state.Get(ctx)
	missing := s.PkgName + "-does-not-exist-at-all"

	var outsider *harborclient.Client
	for _, u := range s.CreatedUsers {
		outsider = s.Client.WithCredentials(u.Name, u.Password)
	}
	if outsider == nil {
		return fmt.Errorf("scenario created no user to probe with")
	}

	probes := []struct {
		who    string
		client func(string) (*harborclient.Response, error)
	}{
		{"a non-member", outsider.Get},
		{"no credentials", s.Client.GetAnonymous},
	}
	for _, p := range probes {
		real, err := p.client("/npm/" + s.PkgProject + "/" + s.PkgName)
		if err != nil {
			return err
		}
		fake, err := p.client("/npm/" + s.PkgProject + "/" + missing)
		if err != nil {
			return err
		}
		if real.StatusCode != fake.StatusCode {
			return fmt.Errorf("as %s the response reveals existence: %d for a package that exists, %d for one that does not",
				p.who, real.StatusCode, fake.StatusCode)
		}
		if string(real.Body) != string(fake.Body) {
			return fmt.Errorf("as %s the bodies differ between an existing and a missing package:\nexists: %s\nmissing: %s",
				p.who, truncate(real.Body), truncate(fake.Body))
		}
	}
	return nil
}

// projectHoldsOnlyVersion reads the registry's own view rather than trusting
// that a refused publish stored nothing.
func projectHoldsOnlyVersion(ctx context.Context, projectAlias, want, base string) error {
	s := state.Get(ctx)
	name, err := resolvePkg(s, base)
	if err != nil {
		return err
	}
	project := projectName(s, projectAlias)
	resp, err := s.Client.Get("/npm/" + project + "/" + name)
	if err != nil {
		return err
	}
	if err := harborclient.Expect(resp, 200); err != nil {
		return fmt.Errorf("read packument for %s: %w", name, err)
	}
	var packument struct {
		Versions map[string]json.RawMessage `json:"versions"`
	}
	if err := json.Unmarshal(resp.Body, &packument); err != nil {
		return fmt.Errorf("decode packument: %w", err)
	}
	got := make([]string, 0, len(packument.Versions))
	for v := range packument.Versions {
		got = append(got, v)
	}
	sort.Strings(got)
	if len(got) != 1 || got[0] != want {
		return fmt.Errorf("expected only version %q, found %v", want, got)
	}
	return nil
}

// userCanStillInstall is the control half of a denial: it proves the refusal
// was about the action, not about the account being broken.
func userCanStillInstall(ctx context.Context, alias, base, projectAlias string) error {
	s := state.Get(ctx)
	u, err := userCred(s, alias)
	if err != nil {
		return err
	}
	if _, err := npmInstall(ctx, s, base, projectAlias, true, u.Name, u.Password, ""); err != nil {
		return err
	}
	if s.LastCLIErr != nil {
		return fmt.Errorf("%s could not install, so the earlier refusal proves nothing: %s", alias, cliFailure(s))
	}
	return nil
}
