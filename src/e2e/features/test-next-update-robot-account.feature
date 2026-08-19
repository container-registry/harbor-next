Feature: Robot account update permission restrictions

  Background:
    Given a running Harbor

  @test-next-update-robot-account @robot @fedidp
  Scenario: Federated robot creation strips robot create and update permissions
    Given project federated identity providers are enabled
    And a fresh private project "proj"
    And an offline federated identity provider "idp"
    When the admin creates federated robot "fed" linked to "idp" requesting robot management and pull permission on "proj"
    Then the robot "fed" is associated with identity provider "idp"
    And robot "fed" has robot permission actions "read, list, delete" in "proj"
    And robot "fed" does not have robot create or update permission in "proj"

  @test-next-update-robot-account @robot @fedidp
  Scenario: Federated robot update strips robot create and update permissions
    Given project federated identity providers are enabled
    And a fresh private project "proj"
    And an offline federated identity provider "idp"
    And a federated robot "fed" linked to "idp" with pull permission on "proj"
    When the admin updates federated robot "fed" requesting robot management and pull permission on "proj"
    Then robot "fed" has robot permission actions "read, list, delete" in "proj"
    And robot "fed" does not have robot create or update permission in "proj"

  @test-next-update-robot-account @robot @fedidp
  Scenario: Federated robot cannot create or update robot accounts with JWT authentication
    Given project federated identity providers are enabled
    And a fresh private project "proj"
    And an offline federated identity provider "idp"
    And IdP-level claim "aud" equals "robot-rbac" on "idp"
    And a federated robot "fed" linked to "idp" requesting robot management and pull permission on "proj"
    And robot-level claim "sub" equals "workload" on "fed" for "idp"
    And a project robot "target" has pull permission in "proj"
    When federated robot "fed" attempts to create child robot "child" with pull permission in "proj" using JWT claims aud="robot-rbac", sub="workload" signed by "idp"
    Then the request is forbidden
    When federated robot "fed" attempts to update robot "target" description to "updated by federated robot" using JWT claims aud="robot-rbac", sub="workload" signed by "idp"
    Then the request is forbidden
