Feature: Federated Identity Provider feature gate

  @fedidp @serial
  Scenario: Project-level IdPs are forbidden when the feature gate is disabled
    Given a running Harbor
    And project federated identity providers are disabled until scenario cleanup
    And a fresh private project "gate-disabled-proj"
    When the admin creates a project-level offline identity provider "gate-disabled-idp" on "gate-disabled-proj"
    Then the request is forbidden
    When the admin creates an offline identity provider "gate-disabled-system-idp"
    Then the request is authorized
