Feature: Harbor system health and access boundaries

  Background:
    Given a running Harbor

  @smoke @system
  Scenario: Harbor reports healthy across all components
    When the admin requests system health
    Then every component is reported healthy
    And anonymous system info matches authenticated system info

  @system @security
  Scenario: Admin APIs reject anonymous and invalid credentials
    When an anonymous client lists users
    Then the request is unauthorized
    When a client with invalid credentials lists users
    Then the request is unauthorized

  @system @audit
  Scenario: Admin can filter the audit log by operation
    Given a fresh project
    When the admin lists audit log entries with operation "create"
    Then the response includes at least one entry for the project
    And the response advertises the total count
