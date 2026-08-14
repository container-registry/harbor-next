Feature: Project webhook notifications

  Background:
    Given a running Harbor

  @smoke @webhooks
  Scenario: Push triggers a configured webhook
    Given a fresh project "proj"
    And an in-process webhook listener
    And a webhook policy on "proj" for push events targeting the listener
    When the admin pushes an image to "proj/app:v1"
    Then the listener receives a push event whose digest matches the pushed artifact

  @webhooks
  Scenario: Disabled webhook policy does not fire
    Given a fresh project "proj"
    And an in-process webhook listener
    And a webhook policy on "proj" for push events targeting the listener
    And the policy is disabled
    When the admin pushes an image to "proj/app:v1"
    Then the listener receives no event within 10 seconds

  @webhooks
  Scenario: Webhook delivery history records a successful job
    Given a fresh project "proj"
    And an in-process webhook listener
    And a webhook policy on "proj" for push events targeting the listener
    When the admin pushes an image to "proj/app:v1"
    And the listener receives the event
    Then the webhook delivery history lists at least one successful job
