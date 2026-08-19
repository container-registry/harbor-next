Feature: Native package publishing and consumption

  Areas A (publishing and consuming) and B (version promises) of the
  multi-format package registry test plan. Every scenario is written from the
  user's side: a developer publishes, a teammate or a build server consumes,
  each through the ecosystem's own client with no registry-specific plugin.

  Two rules keep these honest. Every package name carries the scenario suffix,
  so a cached copy from an earlier run can never make a scenario pass; and
  where the client refuses an action locally, the step talks to the registry
  directly, because a client-side refusal proves nothing about the server.

  Background:
    Given a running Harbor

  @packages @npm @smoke
  Scenario: A developer publishes an npm package and a teammate installs it
    Given a fresh private project "pkgs"
    And an npm package "acme-lib" at version "1.0.0"
    When the package is published to "pkgs"
    Then the publish succeeds
    When a consumer installs "acme-lib" from "pkgs"
    Then the install succeeds
    And the installed package contains the published content

  @packages @npm
  Scenario: A scoped npm package keeps its identity for consumers
    Given a fresh private project "pkgs"
    And an npm package "@acme/widget" at version "1.0.0" published to "pkgs"
    When a consumer installs "@acme/widget" from "pkgs"
    Then the install succeeds
    And the package is stored in "pkgs" with artifact type "NPM"

  @packages @npm @robot
  Scenario: A build server installs a package with a robot account
    Given a fresh private project "pkgs"
    And an npm package "ci-lib" at version "1.0.0" published to "pkgs"
    And a robot with pull permission on "pkgs"
    When the robot installs "ci-lib" from "pkgs"
    Then the install succeeds

  @packages @npm @robot
  Scenario: A pull-only robot cannot publish
    Given a fresh private project "pkgs"
    And an npm package "ci-lib" at version "1.0.0"
    And a robot with pull permission on "pkgs"
    When the robot publishes the package to "pkgs"
    Then the publish is refused
    And nothing is stored in "pkgs"

  @packages @maven
  Scenario: A Java team deploys a library and another project resolves it
    Given a fresh private project "pkgs"
    And a Maven artifact "com.acme:demo" at version "1.0.0"
    When the artifact is deployed to "pkgs"
    Then the deploy succeeds
    When a consumer project resolves "com.acme:demo" from "pkgs"
    Then the resolve succeeds
    And the package is stored in "pkgs" with artifact type "MAVEN"

  @packages @npm @immutability
  Scenario: A published version can never be replaced
    Given a fresh private project "pkgs"
    And an npm package "locked-lib" at version "1.0.0" published to "pkgs"
    When the same version is published again with different content
    Then the publish is rejected as a conflict
    When a consumer installs "locked-lib" from "pkgs"
    Then the installed package contains the published content

  @packages @npm
  Scenario: Installing without a version gives the newest release
    Given a fresh private project "pkgs"
    And an npm package "range-lib" at version "1.0.0" published to "pkgs"
    And version "1.1.0" of "range-lib" published to "pkgs"
    When a consumer installs "range-lib" from "pkgs" without specifying a version
    Then the installed version is "1.1.0"

  @packages @npm
  Scenario: A lower version published last does not become the default
    Given a fresh private project "pkgs"
    And an npm package "order-lib" at version "1.1.0" published to "pkgs"
    And version "1.0.0" of "order-lib" published to "pkgs" under tag "old"
    When a consumer installs "order-lib" from "pkgs" without specifying a version
    Then the installed version is "1.1.0"
