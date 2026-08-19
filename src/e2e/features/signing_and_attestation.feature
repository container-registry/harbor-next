Feature: Image signing and attestation accessories

  Background:
    Given a running Harbor

  @smoke @signing @sigstore
  Scenario: Cosign signature appears as an accessory and verifies
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    And a freshly generated cosign key pair
    When the admin signs "proj/app:v1" with the cosign key
    Then "proj/app:v1" has a cosign signature accessory
    When the accessory is verified with the matching public key
    Then verification passes

  @signing @sigstore
  Scenario: Verification with the wrong key fails
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    And a freshly generated cosign key pair "A"
    And a freshly generated cosign key pair "B"
    When the admin signs "proj/app:v1" with key pair "A"
    And the accessory is verified with the public key of pair "B"
    Then verification fails

  @signing @attestation @buildx
  Scenario: buildx provenance attestation appears as an in-toto accessory
    Given a fresh project "proj"
    And the multi-arch build fixture
    When buildx pushes the fixture to "proj/app:v1" with provenance attestation
    Then "proj/app:v1" has an attestation accessory
    And the attestation payload type is in-toto

  @signing @attestation @sigstore
  Scenario: SBOM attestation round-trips through Harbor
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    And an SPDX JSON SBOM predicate
    And a freshly generated cosign key pair
    When the admin attaches the SBOM as an attestation on "proj/app:v1"
    Then "proj/app:v1" has an SBOM attestation accessory
    And the accessory predicate matches the pushed predicate
