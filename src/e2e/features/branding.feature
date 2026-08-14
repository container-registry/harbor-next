Feature: System branding configuration (patch 0001)
  As a Harbor administrator
  I want to customise the product branding exposed by the API
  So that self-hosted deployments can present their own identity

  # Patch 0001 adds GET/POST /api/v2.0/systeminfo/branding
  # GET is public; POST is system-admin only.

  @smoke @branding
  Scenario: Admin reads the default branding configuration
    When I request the branding configuration
    Then the response status is 200
    And the branding config contains a "product" field

  @branding
  Scenario: Admin updates the branding product name and reads it back
    When I request the branding configuration
    And I update the branding product name to "8GCR Commercial"
    Then the response status is 200
    When I request the branding configuration
    Then the branding product name is "8GCR Commercial"

  @branding
  Scenario: Non-admin user cannot update branding
    Given a fresh user "branding-dev"
    When I update the branding product name to "Hacked" as user "branding-dev"
    Then the response status is 403
