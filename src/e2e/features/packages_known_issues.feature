Feature: Native package behaviour that is not implemented yet

  Scenarios describing behaviour the multi-format registry does not have today.
  Each one is a defect found while executing areas A and B of the test plan,
  written as the outcome a user expects rather than the outcome they get.

  They are tagged @known-issue and excluded from the default lane, so a red
  default build always means a regression. Run them deliberately with:

      task e2e TAGS='@known-issue'

  Each scenario turns green by itself when the corresponding fix lands. None of
  them should be deleted to make a build pass.

  Background:
    Given a running Harbor

  # npm sends only {"beta": "2.0.0-beta.1"} when publishing with --tag beta,
  # and the dist-tags endpoint returns exactly that. The packument, however, is
  # rendered with a "latest" fallback of semver max over every version — and
  # that maximum does not exclude pre-releases. So every consumer who asks for
  # no version silently moves to the beta, while the Portal still shows only
  # the beta tag and looks correct.
  # src/server/registry/npm/handler.go, packument dist-tags fallback.
  @packages @npm @known-issue
  Scenario: A pre-release does not reach consumers who ask for no version
    Given a fresh private project "pkgs"
    And an npm package "beta-lib" at version "1.1.0" published to "pkgs"
    And version "2.0.0-beta.1" of "beta-lib" published to "pkgs" under tag "beta"
    When a consumer installs "beta-lib" from "pkgs" without specifying a version
    Then the installed version is "1.1.0"

  # The repository list renders the OCI storage path (pkgs/npm/acme/widget)
  # instead of the package coordinate (@acme/widget). The detail page and the
  # Usage tab both render the native name, so the model holds it and only the
  # list view is wrong. The same applies to Maven, which lists a path rather
  # than a group:artifact coordinate.
  @packages @npm @known-issue
  Scenario: The repository listing shows the npm package name
    Given a fresh private project "pkgs"
    And an npm package "@acme/widget" at version "1.0.0" published to "pkgs"
    When the repositories under "pkgs" are listed
    Then the listed package name is "@acme/widget"

  # The Portal's Usage tab hands the user an .npmrc built around _authToken.
  # A robot secret used that way is rejected; the same secret works as basic
  # auth. docs/artifact-types/npm.md, shipped in this same change, already says
  # tokens are not honoured — so the screen recommends the one method the
  # documentation calls broken.
  @packages @npm @robot @known-issue
  Scenario: A robot secret authenticates as an npm auth token
    Given a fresh private project "pkgs"
    And an npm package "token-lib" at version "1.0.0" published to "pkgs"
    And a robot with pull permission on "pkgs"
    When the robot installs "token-lib" from "pkgs" with token authentication
    Then the install succeeds
