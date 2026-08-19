Feature: Access control for native packages

  Area C of the multi-format package registry test plan. Two questions: does
  private content stay private, and does a machine credential stay inside the
  project it was issued for.

  Every denial here is paired with the same action succeeding for someone
  entitled to it. Without that pair a refusal proves nothing — a feature that
  is broken for everybody refuses everybody, and the scenario still passes.

  Background:
    Given a running Harbor

  @packages @npm @rbac
  Scenario: A package in a private project is invisible to an outsider
    Given a fresh private project "pkgs"
    And an npm package "team-lib" at version "1.0.0" published to "pkgs"
    And a fresh user "outsider" with no project membership
    When "outsider" installs "team-lib" from "pkgs"
    Then the install is refused
    And the refusal does not reveal whether the package exists

  @packages @npm @rbac
  Scenario: A read-only member cannot publish
    Given a fresh private project "pkgs"
    And an npm package "team-lib" at version "1.0.0" published to "pkgs"
    And a fresh user "reader" with the "guest" role on "pkgs"
    When "reader" publishes version "1.0.1" of "team-lib" to "pkgs"
    Then the publish is refused
    And "pkgs" holds only version "1.0.0" of "team-lib"
    And "reader" can still install "team-lib" from "pkgs"

  @packages @npm @robot
  Scenario: A robot cannot reach beyond the project it was issued for
    Given a fresh private project "pkgs"
    And a fresh private project "other"
    And an npm package "team-lib" at version "1.0.0"
    And a robot with push and pull permission on "pkgs"
    When the robot publishes the package to "other"
    Then the publish is refused
    And nothing is stored in "other"
    When the robot publishes the package to "pkgs"
    Then the publish succeeds

  @packages @npm @robot
  Scenario: Disabling a robot stops access immediately
    Given a fresh private project "pkgs"
    And an npm package "team-lib" at version "1.0.0" published to "pkgs"
    And a robot with pull permission on "pkgs"
    When the robot installs "team-lib" from "pkgs"
    Then the install succeeds
    When the robot is disabled
    And the robot installs "team-lib" from "pkgs"
    Then the install is refused

  @packages @npm @robot
  Scenario: Deleting a robot stops access immediately
    Given a fresh private project "pkgs"
    And an npm package "team-lib" at version "1.0.0" published to "pkgs"
    And a robot with pull permission on "pkgs"
    When the robot installs "team-lib" from "pkgs"
    Then the install succeeds
    When the robot is deleted
    And the robot installs "team-lib" from "pkgs"
    Then the install is refused
