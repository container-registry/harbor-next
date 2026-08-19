Feature: Project lifecycle and role-based access

  Background:
    Given a running Harbor

  @smoke @projects
  Scenario: Project admin configures a project end-to-end and delete cascades
    Given a fresh project "alpha"
    When the admin sets the storage quota on "alpha" to 1 GiB
    And the admin adds a label "team-x" to "alpha"
    And the admin registers a webhook on "alpha" listening for push events
    Then the quota, label, and webhook are visible on "alpha"
    When the admin deletes "alpha"
    Then the project and its label and webhook are gone

  @projects
  Scenario: Duplicate project names are rejected
    Given a fresh project "alpha"
    When the admin attempts to create another project named "alpha"
    Then the request is rejected as a conflict

  @rbac
  Scenario: A developer can push but not delete, and loses access when removed
    Given a fresh private project "proj"
    And a fresh user "dev"
    And "dev" is assigned the "Developer" role on "proj"
    When "dev" pushes an image to "proj/app:v1"
    Then the push succeeds
    When "dev" attempts to delete "proj"
    Then the request is forbidden
    When the admin removes "dev" from "proj"
    Then "dev" can no longer push to "proj"

  @smoke @rbac @robot
  Scenario: A project-scoped robot can push and pull only within its project
    Given a fresh project "proj-a"
    And a fresh project "proj-b"
    And a robot with push and pull permission on "proj-a"
    When the robot pushes an image to "proj-a/app:v1"
    Then the push succeeds
    When the robot pulls "proj-a/app:v1"
    Then the pull succeeds
    When the robot pushes an image to "proj-b/app:v1"
    Then the push is forbidden
    When the admin deletes the robot
    Then the robot credentials are rejected as unauthorized

  @rbac @projects
  Scenario: A private project is invisible to non-members
    Given a fresh private project "secret"
    And a fresh user "outsider" with no project membership
    When "outsider" lists projects
    Then "secret" is not in the response
    When "outsider" requests "secret" directly
    Then the request is forbidden
