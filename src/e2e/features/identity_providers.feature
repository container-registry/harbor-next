Feature: Federated Identity Provider (FedIDP) API

  Background:
    Given a running Harbor
    And project federated identity providers are enabled

  # ─────────────────────────────────────────────
  # CRUD lifecycle
  # ─────────────────────────────────────────────

  @smoke @fedidp
  Scenario: FedIDP CRUD lifecycle
    When the admin creates an offline identity provider "my-idp"
    Then the identity provider "my-idp" is returned by the API
    When the admin updates identity provider "my-idp" description to "updated desc"
    Then the identity provider "my-idp" has description "updated desc"
    When the admin deletes identity provider "my-idp"
    Then getting identity provider "my-idp" returns not found

  @fedidp
  Scenario: Identity providers are listed and paginated
    When the admin creates an offline identity provider "idp-list-a"
    And the admin creates an offline identity provider "idp-list-b"
    Then the identity provider list includes "idp-list-a"
    And the identity provider list includes "idp-list-b"
    And the identity provider list pagination returns one item per page

  # ─────────────────────────────────────────────
  # Input validation
  # ─────────────────────────────────────────────

  @fedidp
  Scenario: Creating a FedIDP with an empty name is rejected
    When the admin creates an identity provider with name "" and issuer "https://test.example.com"
    Then the request is rejected as bad request

  @fedidp
  Scenario: Creating a FedIDP with a whitespace-only name is rejected
    When the admin creates an identity provider with name "   " and issuer "https://test.example.com"
    Then the request is rejected as bad request

  @fedidp
  Scenario: Creating a FedIDP with a very long name is rejected
    When the admin creates an identity provider with a 500-character name
    Then the request is rejected as bad request

  @fedidp
  Scenario: Creating a FedIDP with an empty issuer is rejected
    When the admin creates an identity provider with name "valid-name" and issuer ""
    Then the request is rejected as bad request

  @fedidp
  Scenario: Duplicate FedIDP name is rejected with 409
    When the admin creates an offline identity provider "dup-idp"
    And the admin creates an offline identity provider "dup-idp"
    Then the request is rejected as a conflict

  # ─────────────────────────────────────────────
  # Claims API
  # ─────────────────────────────────────────────

  @fedidp
  Scenario: Claims can be added, listed, and deleted
    When the admin creates an offline identity provider "claims-idp"
    And the admin adds claim "aud" equal to "my-audience" to "claims-idp"
    And the admin adds claim "iss" equal to "https://issuer.example.com" to "claims-idp"
    Then the identity provider "claims-idp" has 2 claims
    When the admin deletes claim "aud" from "claims-idp"
    Then the request is rejected as bad request
    And the identity provider "claims-idp" has 2 claims

  @fedidp @claim-rule-path-scope
  Scenario: Claim rule mutations are scoped by the path identity provider
    When the admin creates an offline identity provider "claim-route-scope-idp"
    And the admin adds claim "sub" equal to "route-scope-workload" to "claim-route-scope-idp"
    Then the response status is 201
    And the identity provider "claim-route-scope-idp" has IdP-level claim "sub" equal to "route-scope-workload"
    When the admin adds a claim using identity provider ID 0
    Then the request is rejected as bad request
    When the admin adds a claim using identity provider ID 999999999
    Then the request returns not found
    When the admin requests system health
    Then the response status is 200
    And every component is reported healthy

  @fedidp
  Scenario: Duplicate claim on the same IdP is rejected with 409
    When the admin creates an offline identity provider "dup-claim-idp"
    And the admin adds claim "sub" equal to "alice" to "dup-claim-idp"
    And the admin adds claim "sub" equal to "alice" to "dup-claim-idp"
    Then the request is rejected as a conflict

  @fedidp
  Scenario: Deleting a non-existent claim returns not found
    When the admin creates an offline identity provider "del-claim-idp"
    Then deleting claim 999999 from "del-claim-idp" returns not found

  @fedidp
  Scenario: System-level IdP persists inherited claim rules
    When the admin creates an offline identity provider "system-claims-idp"
    And the admin adds claim "aud" equal to "harbor" to "system-claims-idp"
    Then the identity provider "system-claims-idp" has IdP-level claim "aud" equal to "harbor"

  @fedidp
  Scenario: Omitted claims_supported is returned as empty and does not restrict claim paths
    Given a fresh project "optional-claims-proj"
    When the admin creates an offline identity provider "optional-claims-idp"
    Then the identity provider "optional-claims-idp" has no supported claims
    When the admin creates a federated robot "optional-claims-robot" linked to "optional-claims-idp" with pull permission on "optional-claims-proj"
    And the admin adds robot claim "custom.claim" equal to "custom-value" to "optional-claims-robot" on "optional-claims-idp"
    Then the identity provider "optional-claims-idp" has robot-level claim "custom.claim" equal to "custom-value" for "optional-claims-robot"

  # ─────────────────────────────────────────────
  # Robot + IdP association
  # ─────────────────────────────────────────────

  @smoke @fedidp
  Scenario: Robot can be linked to a FedIDP
    Given a fresh project "fedidp-proj"
    When the admin creates an offline identity provider "robot-idp"
    And the admin adds claim "aud" equal to "harbor" to "robot-idp"
    And the admin creates a federated robot "fed-robot" linked to "robot-idp" with pull permission on "fedidp-proj"
    Then the robot "fed-robot" is associated with identity provider "robot-idp"

  @fedidp
  Scenario: Robot-level claim can be added to a FedIDP robot
    Given a fresh project "claim-proj"
    When the admin creates an offline identity provider "claim-robot-idp"
    And the admin creates a federated robot "claim-robot" linked to "claim-robot-idp" with pull permission on "claim-proj"
    And the admin adds robot claim "sub" equal to "system:serviceaccount:default:default" to "claim-robot" on "claim-robot-idp"
    Then the identity provider "claim-robot-idp" has 1 claims

  @fedidp
  Scenario: Federated robot creation does not expose or accept a static secret
    Given a fresh private project "secretless-proj"
    And an offline federated identity provider "secretless-idp"
    When the admin creates a federated robot "secretless-robot" linked to "secretless-idp" with pull permission on "secretless-proj"
    Then the created federated robot response exposes no static secret
    When the federated robot static secret is used to request the Harbor API
    Then the request is unauthorized

  @fedidp
  Scenario: Duplicate exact federated robot claim set is rejected
    Given a fresh private project "dup-set-proj"
    And an offline federated identity provider "dup-set-idp"
    And IdP-level claim "aud" equals "dup-set-proj" on "dup-set-idp"
    And a federated robot "dup-set-robot-a" linked to "dup-set-idp" with pull permission on "dup-set-proj"
    And robot-level claim "sub" equals "same-workload" on "dup-set-robot-a" for "dup-set-idp"
    And a federated robot "dup-set-robot-b" linked to "dup-set-idp" with pull permission on "dup-set-proj"
    When the admin tries to add robot claim "sub" equal to "same-workload" to "dup-set-robot-b" on "dup-set-idp"
    Then the request is rejected as a conflict

  # ─────────────────────────────────────────────
  # JWT authentication (offline / in-process IdP)
  # ─────────────────────────────────────────────

  @smoke @fedidp
  Scenario: Robot authenticates to Harbor using a JWT from the offline IdP
    Given a fresh private project "jwt-proj"
    And an offline federated identity provider "jwt-idp"
    And IdP-level claim "aud" equals "jwt-proj" on "jwt-idp"
    And a federated robot "jwt-robot" linked to "jwt-idp" with pull permission on "jwt-proj"
    And robot-level claim "sub" equals "ci-workload" on "jwt-robot" for "jwt-idp"
    When the robot presents a JWT with claims aud="jwt-proj", sub="ci-workload" signed by "jwt-idp"
    Then the request is authorized

  @smoke @fedidp
  Scenario: Credential-provider Basic JWT flow returns a registry bearer token
    Given a fresh private project "basic-jwt-proj"
    And an offline federated identity provider "basic-jwt-idp"
    And IdP-level claim "aud" equals "basic-jwt-proj" on "basic-jwt-idp"
    And a federated robot "basic-jwt-robot" linked to "basic-jwt-idp" with pull permission on "basic-jwt-proj"
    And robot-level claim "sub" equals "kubelet-workload" on "basic-jwt-robot" for "basic-jwt-idp"
    When the credential provider requests a registry token for "basic-jwt-proj/app" with claims aud="basic-jwt-proj", sub="kubelet-workload" signed by "basic-jwt-idp"
    Then the request is authorized
    And the registry token response contains a bearer token

  @fedidp
  Scenario: JWT with wrong issuer is rejected
    Given a fresh private project "bad-iss-proj"
    And an offline federated identity provider "bad-iss-idp"
    And IdP-level claim "aud" equals "bad-iss-proj" on "bad-iss-idp"
    And a federated robot "bad-iss-robot" linked to "bad-iss-idp" with pull permission on "bad-iss-proj"
    And robot-level claim "sub" equals "ci-workload" on "bad-iss-robot" for "bad-iss-idp"
    When the robot presents a JWT signed by a different issuer
    Then the request is unauthorized

  @fedidp
  Scenario: Expired JWT is rejected
    Given a fresh private project "exp-proj"
    And an offline federated identity provider "exp-idp"
    And IdP-level claim "aud" equals "exp-proj" on "exp-idp"
    And a federated robot "exp-robot" linked to "exp-idp" with pull permission on "exp-proj"
    And robot-level claim "sub" equals "ci-workload" on "exp-robot" for "exp-idp"
    When the robot presents an expired JWT signed by "exp-idp" with claims aud="exp-proj", sub="ci-workload"
    Then the request is unauthorized

  @fedidp
  Scenario: Tampered JWT signature is rejected
    Given a fresh private project "tamper-proj"
    And an offline federated identity provider "tamper-idp"
    And IdP-level claim "aud" equals "tamper-proj" on "tamper-idp"
    And a federated robot "tamper-robot" linked to "tamper-idp" with pull permission on "tamper-proj"
    And robot-level claim "sub" equals "ci-workload" on "tamper-robot" for "tamper-idp"
    When the robot presents a tampered JWT from "tamper-idp" with claims aud="tamper-proj", sub="ci-workload"
    Then the request is unauthorized

  @fedidp
  Scenario: JWT with mismatched claim value is rejected
    Given a fresh private project "mismatch-proj"
    And an offline federated identity provider "mismatch-idp"
    And IdP-level claim "aud" equals "mismatch-proj" on "mismatch-idp"
    And a federated robot "mismatch-robot" linked to "mismatch-idp" with pull permission on "mismatch-proj"
    And robot-level claim "sub" equals "correct-workload" on "mismatch-robot" for "mismatch-idp"
    When the robot presents a JWT with claims aud="mismatch-proj", sub="wrong-workload" signed by "mismatch-idp"
    Then the request is unauthorized

  @fedidp
  Scenario: Array-valued JWT claims match scalar robot claim rules
    Given a fresh private project "array-claim-proj"
    And an offline federated identity provider "array-claim-idp"
    And IdP-level claim "aud" equals "array-claim-proj" on "array-claim-idp"
    And a federated robot "array-claim-robot" linked to "array-claim-idp" with pull permission on "array-claim-proj"
    And robot-level claim "groups" equals "harbor-pullers" on "array-claim-robot" for "array-claim-idp"
    When the robot presents a JWT with claims aud="array-claim-proj", sub="array-workload", groups containing "harbor-pullers" signed by "array-claim-idp"
    Then the request is authorized

  @fedidp
  Scenario: Kubernetes nested JWT claim paths match direct dot claim rules
    Given a fresh private project "nested-claim-proj"
    And an offline federated identity provider "nested-claim-idp"
    And IdP-level claim "aud" equals "nested-claim-proj" on "nested-claim-idp"
    And a federated robot "nested-claim-robot" linked to "nested-claim-idp" with pull permission on "nested-claim-proj"
    And robot-level claim "kubernetes.io.serviceaccount.name" equals "default" on "nested-claim-robot" for "nested-claim-idp"
    When the robot presents a JWT with Kubernetes service account name "default" and aud="nested-claim-proj" signed by "nested-claim-idp"
    Then the request is authorized

  # ─────────────────────────────────────────────
  # Claim matching (most-specific wins)
  # ─────────────────────────────────────────────

  @fedidp
  Scenario: Most-specific robot wins when two robots share an IdP
    Given a fresh private project "match-broad-proj"
    And a fresh private project "match-specific-proj"
    And an offline federated identity provider "match-idp"
    And IdP-level claim "aud" equals "match-aud" on "match-idp"
    And a federated robot "broad-robot" linked to "match-idp" with pull permission on "match-broad-proj"
    And robot-level claim "sub" equals "team-a" on "broad-robot" for "match-idp"
    And a federated robot "specific-robot" linked to "match-idp" with pull permission on "match-specific-proj"
    And robot-level claim "team" equals "backend" on "specific-robot" for "match-idp"
    And robot-level claim "sub" equals "team-a" on "specific-robot" for "match-idp"
    When the robot presents a JWT with claims aud="match-aud", sub="team-a", team="backend" signed by "match-idp"
    Then the request is authorized

  # ─────────────────────────────────────────────
  # Uniqueness and project scope
  # ─────────────────────────────────────────────

  @fedidp @idp-audience-routing
  Scenario: Project IdPs can share an issuer with distinct audiences
    Given a fresh private project "issuer-audience-project-a"
    And a fresh private project "issuer-audience-project-b"
    When the admin creates a project-level offline identity provider "issuer-audience-idp-a" on "issuer-audience-project-a"
    And the admin creates a project-level offline identity provider "issuer-audience-idp-b" on "issuer-audience-project-b" with the issuer from "issuer-audience-idp-a"
    And the admin adds claim "aud" equal to "issuer-audience-a" to "issuer-audience-idp-a"
    And the admin adds claim "aud" equal to "issuer-audience-b" to "issuer-audience-idp-b"
    Then the identity provider "issuer-audience-idp-a" has IdP-level claim "aud" equal to "issuer-audience-a"
    And the identity provider "issuer-audience-idp-b" has IdP-level claim "aud" equal to "issuer-audience-b"

  @fedidp @idp-audience-routing
  Scenario: Project IdPs cannot share an issuer and audience
    Given a fresh private project "issuer-audience-conflict-project-a"
    And a fresh private project "issuer-audience-conflict-project-b"
    When the admin creates a project-level offline identity provider "issuer-audience-conflict-idp-a" on "issuer-audience-conflict-project-a"
    And the admin adds claim "aud" equal to "issuer-audience-shared" to "issuer-audience-conflict-idp-a"
    And the admin creates a project-level offline identity provider "issuer-audience-conflict-idp-b" on "issuer-audience-conflict-project-b" with the issuer from "issuer-audience-conflict-idp-a"
    And the admin adds claim "aud" equal to "issuer-audience-shared" to "issuer-audience-conflict-idp-b"
    Then the request is rejected as a conflict

  @fedidp
  Scenario: Project-level IdP appears only in project-scoped list
    Given a fresh private project "project-idp-proj"
    When the admin creates a project-level offline identity provider "project-only-idp" on "project-idp-proj"
    Then the project-level identity provider list for "project-idp-proj" includes "project-only-idp"
    And the identity provider list excludes "project-only-idp"

  @fedidp
  Scenario: Project-level IdP list validates query shape
    Then listing project-level identity providers without a project ID is rejected as bad request
    And listing identity providers with invalid level is rejected as bad request

  @fedidp
  Scenario: Project robot can link to system and same-project IdPs only
    Given a fresh private project "scope-a"
    And a fresh private project "scope-b"
    And an offline federated identity provider "scope-system-idp"
    And a project-level offline federated identity provider "scope-project-a-idp" on "scope-a"
    And a project-level offline federated identity provider "scope-project-b-idp" on "scope-b"
    When the admin creates a federated robot "scope-system-robot" linked to "scope-system-idp" with pull permission on "scope-a"
    Then the robot "scope-system-robot" is associated with identity provider "scope-system-idp"
    When the admin creates a federated robot "scope-project-robot" linked to "scope-project-a-idp" with pull permission on "scope-a"
    Then the robot "scope-project-robot" is associated with identity provider "scope-project-a-idp"
    When the admin tries to create a federated robot "scope-cross-robot" linked to "scope-project-b-idp" with pull permission on "scope-a"
    Then the request is forbidden

  @fedidp
  Scenario: System robot cannot link to project-level IdP
    Given a fresh private project "system-scope-proj"
    And a project-level offline federated identity provider "system-scope-project-idp" on "system-scope-proj"
    When the admin tries to create a system federated robot "system-scope-robot" linked to "system-scope-project-idp"
    Then the request is rejected as bad request

  # ─────────────────────────────────────────────
  # Delete and update safety
  # ─────────────────────────────────────────────

  @fedidp
  Scenario: IdP with linked robot cannot be deleted until the robot is removed
    Given a fresh private project "delete-safe-proj"
    And an offline federated identity provider "delete-safe-idp"
    And a federated robot "delete-safe-robot" linked to "delete-safe-idp" with pull permission on "delete-safe-proj"
    When the admin deletes identity provider "delete-safe-idp"
    Then the request is rejected as bad request with message containing "delete the associated robots"
    When the admin deletes federated robot "delete-safe-robot"
    Then the robot "delete-safe-robot" is not associated with identity provider "delete-safe-idp"
    When the admin deletes identity provider "delete-safe-idp"
    Then getting identity provider "delete-safe-idp" returns not found

  @fedidp
  Scenario: Immutable offline IdP fields cannot be changed
    When the admin creates an offline identity provider "immutable-idp"
    And the admin updates identity provider "immutable-idp" name to "immutable-new-name"
    Then the request is rejected as bad request
    When the admin updates identity provider "immutable-idp" issuer to "https://new-issuer.example.test"
    Then the request is rejected as bad request
    When the admin switches identity provider "immutable-idp" to online validation
    Then the request is rejected as bad request

  @fedidp
  Scenario: Offline JWKS rotation invalidates old tokens and accepts new tokens
    Given a fresh private project "rotate-proj"
    And an offline federated identity provider "rotate-idp"
    And IdP-level claim "aud" equals "rotate-proj" on "rotate-idp"
    And a federated robot "rotate-robot" linked to "rotate-idp" with pull permission on "rotate-proj"
    And robot-level claim "sub" equals "rotate-workload" on "rotate-robot" for "rotate-idp"
    When the robot presents a JWT with claims aud="rotate-proj", sub="rotate-workload" signed by "rotate-idp"
    Then the request is authorized
    When the admin updates identity provider "rotate-idp" JWKS keys to a new key
    And the same JWT is used to request the Harbor API
    Then the request is unauthorized
    When the robot presents a JWT with claims aud="rotate-proj", sub="rotate-workload" signed by "rotate-idp"
    Then the request is authorized

  # ─────────────────────────────────────────────
  # Additional auth and claims validation
  # ─────────────────────────────────────────────

  @fedidp
  Scenario: Unsupported JWT algorithm and unknown key are rejected
    Given a fresh private project "jwt-security-proj"
    And an offline federated identity provider "jwt-security-idp"
    And IdP-level claim "aud" equals "jwt-security-proj" on "jwt-security-idp"
    And a federated robot "jwt-security-robot" linked to "jwt-security-idp" with pull permission on "jwt-security-proj"
    And robot-level claim "sub" equals "jwt-security-workload" on "jwt-security-robot" for "jwt-security-idp"
    When the robot presents an unsupported-algorithm JWT with claims aud="jwt-security-proj", sub="jwt-security-workload" signed by "jwt-security-idp"
    Then the request is unauthorized
    When the robot presents a JWT with unknown kid and claims aud="jwt-security-proj", sub="jwt-security-workload" signed by "jwt-security-idp"
    Then the request is unauthorized

  @fedidp
  Scenario: Missing required IdP and robot claims are rejected
    Given a fresh private project "missing-claims-proj"
    And an offline federated identity provider "missing-claims-idp"
    And IdP-level claim "aud" equals "missing-claims-proj" on "missing-claims-idp"
    And a federated robot "missing-claims-robot" linked to "missing-claims-idp" with pull permission on "missing-claims-proj"
    And robot-level claim "sub" equals "missing-claims-workload" on "missing-claims-robot" for "missing-claims-idp"
    When the robot presents a JWT missing aud with sub="missing-claims-workload" signed by "missing-claims-idp"
    Then the request is unauthorized
    When the robot presents a JWT missing sub with aud="missing-claims-proj" signed by "missing-claims-idp"
    Then the request is unauthorized

  @fedidp
  Scenario: Claim API filters return exact subsets
    Given a fresh private project "claim-filter-proj"
    And an offline federated identity provider "claim-filter-idp"
    And IdP-level claim "aud" equals "claim-filter-proj" on "claim-filter-idp"
    And a federated robot "claim-filter-robot" linked to "claim-filter-idp" with pull permission on "claim-filter-proj"
    And robot-level claim "sub" equals "claim-filter-workload" on "claim-filter-robot" for "claim-filter-idp"
    Then the identity provider "claim-filter-idp" has 2 claims
    And the identity provider "claim-filter-idp" has 1 claims when listed with "idp_only=true"
    And the identity provider "claim-filter-idp" has 1 claims when listed with "robot_id=0"
    And the identity provider "claim-filter-idp" has 1 claims when listed with "claim_path=sub"
    And the identity provider "claim-filter-idp" has 1 claims when listed with "robot_id=0&claim_path=aud"
    And the identity provider "claim-filter-idp" has 1 claims when listed with "idp_only=true&claim_path=aud"
    When the admin lists claims for "claim-filter-idp" with "idp_only=true&robot_id=0"
    Then the request is rejected as bad request

  @fedidp
  Scenario: Claim validation rejects malformed rules
    When the admin creates an offline identity provider "claim-validation-idp"
    And the admin adds an empty claim path equal to "value" to "claim-validation-idp"
    Then the request is rejected as bad request
    When the admin adds claim "sub" with an empty value to "claim-validation-idp"
    Then the request is rejected as bad request
    When the admin adds a too-long claim path to "claim-validation-idp"
    Then the request is rejected as bad request
    When the admin adds claim "sub" with a too-long value to "claim-validation-idp"
    Then the request is rejected as bad request
    When the admin adds duplicate claim rules in one batch to "claim-validation-idp"
    Then the request is rejected as a conflict

  # ─────────────────────────────────────────────
  # Online OIDC
  # ─────────────────────────────────────────────

  @fedidp
  Scenario: HTTPS online IdP discovery is accepted and HTTP is rejected
    When the admin creates an HTTPS online identity provider "online-https-idp"
    Then the online identity provider "online-https-idp" has derived OpenID fields
    When the admin creates an online identity provider "online-http-idp" from an HTTP discovery URL
    Then the request is rejected as bad request

  @fedidp
  Scenario: Online IdP discovery validation rejects inconsistent metadata
    When the admin creates an online identity provider "online-jwks-mismatch" with mismatched JWKS URI
    Then the request is rejected as bad request
    When the admin creates an online identity provider "online-missing-issuer" from discovery missing issuer
    Then the request is rejected as bad request
    When the admin creates an online identity provider "online-missing-jwks" from discovery missing JWKS URI
    Then the request is rejected as bad request
    When the admin creates an online identity provider "online-invalid-json" from invalid JSON discovery
    Then the request is rejected as bad request

  @fedidp
  Scenario: Online IdP ping endpoints fetch HTTPS discovery and JWKS
    When the admin pings the HTTPS online discovery URL
    Then the request is authorized
    When the admin pings the HTTPS online JWKS URL
    Then the request is authorized
