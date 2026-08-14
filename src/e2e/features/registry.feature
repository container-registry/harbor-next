Feature: Container image push, pull, and storage lifecycle

  Background:
    Given a running Harbor

  @smoke @registry
  Scenario: Single-arch image round-trips through Harbor
    Given a fresh project "proj"
    When the admin pushes a synthetic image to "proj/app:v1"
    Then the artifact and tag "v1" appear under "proj/app"
    When the admin pulls "proj/app:v1"
    Then the pulled digest matches the pushed digest

  @registry @multiarch @buildx
  Scenario: Multi-arch buildx image is a manifest index with pullable platforms
    Given a fresh project "proj"
    And the multi-arch build fixture
    When buildx pushes the fixture to "proj/app:v1" for "linux/amd64,linux/arm64"
    Then "proj/app:v1" is a manifest index with exactly 2 child manifests
    And the child manifests advertise platforms "linux/amd64" and "linux/arm64"
    When each platform manifest is pulled
    Then every pull succeeds

  @registry
  Scenario: A second tag on the same image can be deleted without affecting the first
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin tags the image as "proj/app:v1-copy"
    And the admin deletes tag "v1-copy"
    Then tag "v1" still resolves to the original manifest

  @registry @gc
  Scenario: Deleting the last tag and running GC removes the blob
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin deletes all tags under "proj/app"
    And the admin triggers an immediate garbage collection
    Then the GC execution completes successfully
    And pulling the image by digest returns not found

  @registry @quota
  Scenario: Storage quota is enforced at push time
    Given a fresh project "proj" with 1 MiB storage quota
    When the admin pushes a 600 KiB image to "proj/app:v1"
    Then the push succeeds
    When the admin pushes a second 600 KiB image to "proj/app:v2"
    Then the push is rejected as quota exceeded

  @registry @immutability
  Scenario: Immutability rule blocks overwriting a matched tag
    Given a fresh project "proj"
    And an immutability rule on "proj" matching tags "v*"
    And an image pushed to "proj/app:v1"
    When the admin pushes a different image to "proj/app:v1"
    Then the push is rejected as immutable
