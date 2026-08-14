Feature: Commercial configuration (patch 0004)
  As a Harbor administrator
  I want Commercial settings to expose project identity provider controls
  So that the portal can render the Commercial tab without missing config fields

  Background:
    Given a running Harbor

  @smoke @fedidp @commercial
  Scenario: Commercial configuration exposes project identity provider controls
    When the admin opens the Commercial configuration
    Then project-level identity providers are configurable
