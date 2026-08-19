# ADR: Federated Identity Provider (Workload Identity Federation)

| Attribute | Value |
|-----------|-------|
| **Title** | Federated Identity Provider (Workload Identity Federation) |
| **Status** | Implemented |
| **Authors** | Harbor Community |
| **Date** | December 2024 |
| **Version** | 1.0 |
| **Related** | [Kubernetes KEP-4412](https://github.com/kubernetes/enhancements/issues/4412) |

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Background & Context](#3-background--context)
4. [Design Decision](#4-design-decision)
   - 4.1 [Architecture Overview](#41-architecture-overview)
   - 4.2 [Data Model](#42-data-model)
   - 4.3 [Core Concepts](#43-core-concepts)
5. [JWT Authentication Flow](#5-jwt-authentication-flow)
   - 5.1 [Request Flow](#51-request-flow)
   - 5.2 [Claim Matching Algorithm](#52-claim-matching-algorithm)
   - 5.3 [Code Reference](#53-code-reference)
6. [Security & Trust Model](#6-security--trust-model)
   - 6.1 [Trust Boundaries](#61-trust-boundaries)
   - 6.2 [JWT Validation Steps](#62-jwt-validation-steps)
   - 6.3 [Online vs Offline Validation](#63-online-vs-offline-validation)
   - 6.4 [Attack Vectors & Mitigations](#64-attack-vectors--mitigations)
7. [Integration Patterns](#7-integration-patterns)
   - 7.1 [Kubernetes Integration](#71-kubernetes-integration)
   - 7.2 [GitHub Actions Integration](#72-github-actions-integration)
   - 7.3 [GitLab CI Integration](#73-gitlab-ci-integration)
8. [REST API Reference](#8-rest-api-reference)
   - 8.1 [Identity Provider Endpoints](#81-identity-provider-endpoints)
   - 8.2 [Claim Rules Endpoints](#82-claim-rules-endpoints)
   - 8.3 [Utility Endpoints](#83-utility-endpoints)
9. [Robot Account Integration](#9-robot-account-integration)
   - 9.1 [Claim Hierarchy](#91-claim-hierarchy)
   - 9.2 [Claim Set Uniqueness](#92-claim-set-uniqueness)
   - 9.3 [Creating Federated Robots](#93-creating-federated-robots)
10. [Quirks & Gotchas](#10-quirks--gotchas)
    - 10.1 [Validation Rules](#101-validation-rules)
    - 10.2 [Common Mistakes](#102-common-mistakes)
    - 10.3 [Limitations](#103-limitations)
    - 10.4 [Debugging Tips](#104-debugging-tips)
11. [Configuration Options](#11-configuration-options)
12. [Database Schema Reference](#12-database-schema-reference)
13. [Code Architecture](#13-code-architecture)
14. [Appendices](#14-appendices)
    - A. [Sample JWT Payloads](#a-sample-jwt-payloads)
    - B. [OpenID Configuration Examples](#b-openid-configuration-examples)
    - C. [JWKS Examples](#c-jwks-examples)

---

## 1. Executive Summary

Harbor's **Federated Identity Provider (FedIdP)** feature enables workload identity federation, allowing clients to authenticate using short-lived JWT tokens issued by external OIDC-compliant identity providers instead of static robot account secrets. This eliminates the need for long-lived credentials, reduces secret sprawl, and enables workload-specific authentication for CI/CD pipelines, Kubernetes workloads, and cloud-native applications.

**Key Benefits:**
- **Eliminate static secrets**: No more managing, rotating, or securing long-lived robot passwords
- **Ephemeral credentials**: JWT tokens are short-lived (minutes to hours) and automatically expire
- **Workload-specific auth**: Each CI job, Kubernetes pod, or GitHub Actions workflow gets a unique, verifiable identity
- **Zero-trust security**: Tokens are cryptographically signed and validated against JWKS
- **Flexible claim matching**: Map multiple workloads to the same robot account using claim rules

---

## 2. Problem Statement

### Current Pain Points

Traditional Harbor robot accounts rely on static username/password credentials:

1. **Security Risks of Long-Lived Credentials**
   - Robot secrets are often valid for months or years
   - Secrets stored in CI/CD systems, environment variables, or config files are vulnerable to exposure
   - Compromised credentials grant persistent access until manually revoked

2. **Operational Burden**
   - Secret rotation requires updating every system using the credential
   - No audit trail of which specific workload used the credential
   - Difficult to implement least-privilege access per workload

3. **Compliance Challenges**
   - Many security frameworks require short-lived credentials
   - Static secrets don't meet zero-trust architecture requirements
   - No cryptographic proof of workload identity

### The Solution

Federated Identity Providers solve these problems by:
- Accepting JWT tokens from trusted external identity providers
- Mapping token claims to robot accounts via configurable rules
- Eliminating the need to distribute or store static secrets
- Providing cryptographic verification of workload identity

---

## 3. Background & Context

### What is Workload Identity Federation?

**Workload Identity Federation** is a security pattern where:
1. A workload (CI job, Kubernetes pod, serverless function) obtains a short-lived JWT token from its native identity provider
2. The target service (Harbor) validates the JWT signature against the IdP's public keys
3. Claims in the JWT are matched to determine which permissions the workload receives

This eliminates the "secret zero" problem - there's no initial secret needed to bootstrap authentication.

### OIDC & JWT Fundamentals

**OpenID Connect (OIDC)** is an identity layer on top of OAuth 2.0. Key concepts:

- **Issuer (`iss`)**: The authority that issued the token (e.g., `https://token.actions.githubusercontent.com`)
- **Subject (`sub`)**: The entity the token represents (e.g., a specific repository or service account)
- **Audience (`aud`)**: The intended recipient of the token (e.g., Harbor's URL)
- **JWKS (JSON Web Key Set)**: Public keys used to verify token signatures

**JWT Structure:**
```
header.payload.signature
```

Example decoded payload:
```json
{
  "iss": "https://token.actions.githubusercontent.com",
  "sub": "repo:myorg/myrepo:ref:refs/heads/main",
  "aud": "https://harbor.example.com",
  "exp": 1702584000,
  "iat": 1702580400
}
```

### Kubernetes KEP-4412

Kubernetes Enhancement Proposal 4412 standardizes how Kubernetes workloads obtain OIDC tokens for authentication to external services. Key capabilities:
- ServiceAccount tokens with custom audiences
- Token projection into pods
- Configurable token attributes via `kubelet` settings

Harbor's FedIdP feature is designed to work seamlessly with KEP-4412 compliant Kubernetes clusters.

---

## 4. Design Decision

### 4.1 Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              CLIENT WORKLOAD                                │
│  (Kubernetes Pod / GitHub Actions / GitLab CI / Cloud Function)             │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │    1. Obtain JWT Token    │
                    │    from Native IdP        │
                    └─────────────┬─────────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │    2. Send Request with   │
                    │    JWT to Harbor          │
                    │    (Bearer or Basic Auth) │
                    └─────────────┬─────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                            HARBOR CORE                                       │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                    robotjwt Security Handler                           │ │
│  │                                                                        │ │
│  │  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐               │ │
│  │  │ 3. Extract   │──▶│ 4. Parse     │──▶│ 5. Lookup    │               │ │
│  │  │    JWT       │   │    iss/kid   │   │    IdP       │               │ │
│  │  └──────────────┘   └──────────────┘   └──────┬───────┘               │ │
│  │                                               │                        │ │
│  │  ┌──────────────┐   ┌──────────────┐   ┌──────▼───────┐               │ │
│  │  │ 8. Return    │◀──│ 7. Find      │◀──│ 6. Validate  │               │ │
│  │  │    Robot Ctx │   │    Robot     │   │    JWT       │               │ │
│  │  └──────────────┘   └──────────────┘   └──────────────┘               │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                  │                                           │
│                    ┌─────────────▼─────────────┐                            │
│                    │       PostgreSQL          │                            │
│                    │  ┌─────────────────────┐  │                            │
│                    │  │ identity_providers  │  │                            │
│                    │  │ claim_rules         │  │                            │
│                    │  │ robot_identity_     │  │                            │
│                    │  │   providers         │  │                            │
│                    │  └─────────────────────┘  │                            │
│                    └───────────────────────────┘                            │
└─────────────────────────────────────────────────────────────────────────────┘
                                  │
                    ┌─────────────▼─────────────┐
                    │    External Identity      │
                    │    Provider (IdP)         │
                    │    ┌─────────────────┐    │
                    │    │ JWKS Endpoint   │    │   (Online Mode)
                    │    │ /.well-known/   │    │
                    │    │ openid-config   │    │
                    │    └─────────────────┘    │
                    └───────────────────────────┘
```

### 4.2 Data Model

Harbor stores FedIdP configuration in three PostgreSQL tables:

#### `identity_providers` Table

Stores the federated identity provider configurations.

```sql
CREATE TABLE IF NOT EXISTS identity_providers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,                    -- Human-readable name (unique)
    description TEXT,                      -- Optional description
    issuer TEXT NOT NULL,                  -- JWT issuer URL (unique per project)
    openid_config_url TEXT,                -- OpenID discovery endpoint
    offline_validation BOOLEAN NOT NULL DEFAULT FALSE,  -- Online vs Offline mode
    supported_algorithms TEXT,             -- Informational only (not enforced)
    claims_supported TEXT,                 -- Informational only (not enforced)
    jwks_uri TEXT,                         -- JWKS endpoint (online mode)
    jwks_keys JSONB,                       -- Stored JWKS (offline mode)
    project_id INT NOT NULL,               -- 0 = system-level, >0 = project-level
    creation_time TIMESTAMP DEFAULT NOW(),
    update_time TIMESTAMP DEFAULT NOW(),
    UNIQUE (issuer, project_id)
);
```

#### `robot_identity_providers` Table

Join table linking robots to their identity providers.

```sql
CREATE TABLE IF NOT EXISTS robot_identity_providers (
    id SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id INT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
    creation_time TIMESTAMP DEFAULT NOW(),
    UNIQUE (identity_provider_id, robot_id)
);
```

#### `claim_rules` Table

Stores JWT claim validation rules for IdPs and robots.

```sql
CREATE TABLE IF NOT EXISTS claim_rules (
    id SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id INT NOT NULL,                 -- 0 = IdP-level claim, >0 = robot-specific
    claim_path TEXT NOT NULL,              -- Claim key (e.g., "sub", "kubernetes.io.namespace")
    value TEXT,                            -- Expected claim value
    creation_time TIMESTAMP DEFAULT NOW()
);

CREATE INDEX idx_claim_rules_lookup
ON claim_rules (identity_provider_id, claim_path, value, robot_id);
```

#### Entity Relationship Diagram

```
┌─────────────────────────┐       ┌──────────────────────────────┐
│   identity_providers    │       │          robot               │
├─────────────────────────┤       ├──────────────────────────────┤
│ id (PK)                 │       │ id (PK)                      │
│ name                    │       │ name                         │
│ issuer                  │       │ secret (NULL for federated)  │
│ offline_validation      │       │ permissions                  │
│ jwks_uri / jwks_keys    │       │ expires_at                   │
│ project_id              │       │ disabled                     │
└───────────┬─────────────┘       └──────────────┬───────────────┘
            │                                     │
            │    ┌──────────────────────────┐    │
            │    │ robot_identity_providers │    │
            │    ├──────────────────────────┤    │
            └───▶│ identity_provider_id (FK)│◀───┘
                 │ robot_id (FK)            │
                 └────────────┬─────────────┘
                              │
            ┌─────────────────▼─────────────────┐
            │           claim_rules             │
            ├───────────────────────────────────┤
            │ id (PK)                           │
            │ identity_provider_id (FK)         │
            │ robot_id (0=IdP, >0=robot)        │
            │ claim_path                        │
            │ value                             │
            └───────────────────────────────────┘
```

### 4.3 Core Concepts

#### Identity Provider (IdP)

An **Identity Provider** is a trust anchor configuration that tells Harbor:
- Which JWT issuer to trust
- How to validate JWT signatures (JWKS URI or stored keys)
- What base claims are required for ALL tokens from this issuer

**Types:**
- **System-level IdP** (`project_id = 0`): Available to all projects
- **Project-level IdP** (`project_id > 0`): Scoped to a specific project (requires `enable_project_federated_idp` config)

#### Claim Rules

**Claim Rules** define the JWT claims that must match for authentication:

- **IdP-level claims** (`robot_id = 0`): Mandatory base claims ALL tokens must satisfy
- **Robot-level claims** (`robot_id > 0`): Differentiating claims for specific robots

**Example:**
```
IdP-level claims:
  iss = "https://token.actions.githubusercontent.com"
  aud = "https://harbor.example.com"

Robot "github-ci-main" claims:
  repository = "myorg/myrepo"
  ref = "refs/heads/main"

Robot "github-ci-staging" claims:
  repository = "myorg/myrepo"
  ref = "refs/heads/staging"
```

#### Online vs Offline Validation

| Mode | Description | Use Case |
|------|-------------|----------|
| **Online** | Fetches JWKS from IdP's endpoint on every auth request | Internet-connected environments, automatic key rotation |
| **Offline** | Uses stored JWKS keys from database | Air-gapped environments, private clusters |

---

## 5. JWT Authentication Flow

### 5.1 Request Flow

```
┌──────────┐                  ┌──────────┐                  ┌──────────┐
│  Client  │                  │  Harbor  │                  │   IdP    │
└────┬─────┘                  └────┬─────┘                  └────┬─────┘
     │                             │                             │
     │  1. GET /v2/library/nginx/manifests/latest               │
     │     Authorization: Bearer <JWT>                          │
     │────────────────────────────▶│                             │
     │                             │                             │
     │                             │  2. Parse JWT (no validation)
     │                             │     Extract: issuer, audience, kid │
     │                             │                             │
     │                             │  3. SELECT * FROM identity_providers
     │                             │     JOIN IdP-level aud rule  │
     │                             │     WHERE issuer = ? AND audience matches │
     │                             │                             │
     │                             │  4. Fetch JWKS (online mode)│
     │                             │────────────────────────────▶│
     │                             │◀────────────────────────────│
     │                             │     {"keys": [...]}         │
     │                             │                             │
     │                             │  5. Validate JWT signature  │
     │                             │     using JWKS              │
     │                             │                             │
     │                             │  6. Validate IdP-level claims
     │                             │     (robot_id = 0)          │
     │                             │                             │
     │                             │  7. Find matching robot     │
     │                             │     (claim matching algorithm)
     │                             │                             │
     │                             │  8. Check robot status      │
     │                             │     (disabled? expired?)    │
     │                             │                             │
     │◀────────────────────────────│                             │
     │  9. Response (success/fail) │                             │
     │                             │                             │
```

### 5.2 Claim Matching Algorithm

The claim matching algorithm determines which robot account matches an incoming JWT token.

**Core Logic:**
1. **Robot claims ⊆ Token claims**: ALL claims defined for a robot must be present in the token
2. **Token may have extras**: The token can contain additional claims not required by the robot
3. **Most specific wins**: Among matching robots, the one with the most claims is selected

**SQL Implementation** (`dao.go:FindMatchingRobot`):

```sql
SELECT
    cr.robot_id,
    COUNT(*) as total_claims,
    SUM(CASE WHEN (
        (cr.claim_path = 'sub' AND cr.value = 'repo:myorg/myrepo:ref:refs/heads/main')
        OR (cr.claim_path = 'repository' AND cr.value = 'myorg/myrepo')
        -- ... conditions for each token claim
    ) THEN 1 ELSE 0 END) as matched_claims
FROM
    claim_rules cr
WHERE
    cr.identity_provider_id = ?
    AND cr.robot_id > 0              -- Exclude IdP-level claims
GROUP BY
    cr.robot_id
HAVING
    COUNT(*) = SUM(...)              -- ALL robot claims must match
    AND COUNT(*) > 0                 -- Robot must have at least one claim
ORDER BY
    total_claims DESC                -- Most specific match first
LIMIT 1
```

**Example Scenario:**

Token claims:
```json
{
  "iss": "https://token.actions.githubusercontent.com",
  "sub": "repo:myorg/myrepo:ref:refs/heads/main",
  "repository": "myorg/myrepo",
  "ref": "refs/heads/main",
  "actor": "developer"
}
```

Robot configurations:
```
Robot A (2 claims):
  repository = "myorg/myrepo"
  ref = "refs/heads/main"

Robot B (1 claim):
  repository = "myorg/myrepo"
```

**Result:** Robot A is selected (more specific match with 2 claims vs 1)

### 5.3 Code Reference

**Entry Point** (`robotjwt.go:Generate`):

```go
func (r *robotjwt) Generate(req *http.Request) security.Context {
    // Step 1: Extract JWT token from request
    tokenStr := extractJWTToken(req)

    // Step 2: Parse token to get issuer, audience, and kid (without signature validation)
    jwtToken, _ := jwt.Parse(tokenStr, nil)
    issuer, _ := jwtToken.Claims.GetIssuer()
    audiences, _ := jwtToken.Claims.GetAudience()
    kid := jwtToken.Header["kid"].(string)

    // Step 3: Get exactly one federated IdP by issuer and audience
    idps, _ := federatedidp.Ctl.GetIdpByIssuerAndAudience(req.Context(), issuer, audiences)
    if len(idps) != 1 {
        return nil
    }
    idp := idps[0]

    // Step 4: Get JWKS and validate the token
    jwkSet, _ := getJWKSet(req.Context(), idp, kid)
    parsedToken, _ := jwthandler.ParseToken(tokenStr, jwkSet)

    // Step 5: Validate IdP-level claims
    validateIDPClaims(req.Context(), idp.ID, parsedToken)

    // Step 6: Find matching robot account
    robot, _ := findMatchingRobot(req.Context(), idp.ID, tokenClaims)

    // Step 7: Return security context
    return robotCtx.NewSecurityContext(robot)
}
```

**Token Extraction** (supports both Bearer and Basic Auth):

```go
func extractJWTToken(req *http.Request) string {
    // Try Bearer token first: "Authorization: Bearer <JWT>"
    tokenStr := bearerToken(req)
    if tokenStr != "" {
        return tokenStr
    }

    // Try Basic Auth password: "Authorization: Basic base64(jwt:<JWT>)"
    tokenStr = basicAuthToken(req)
    if IsJWT(tokenStr) {
        return tokenStr
    }

    return ""
}
```

---

## 6. Security & Trust Model

### 6.1 Trust Boundaries

**What Harbor Trusts:**
- JWKS public keys from configured IdP endpoints
- Stored JWKS keys (offline mode) managed by administrators
- JWT signatures signed with trusted keys

**What Harbor Validates:**
- JWT cryptographic signature (against JWKS)
- Token expiration (`exp` claim)
- Token not-before time (`nbf` claim)
- Issuer matches configured IdP (`iss` claim)
- All IdP-level claims match exactly
- Robot-level claims match for robot selection

**What Harbor Does NOT Validate:**
- `supported_algorithms` field (informational only, not enforced)
- `claims_supported` field (informational only, not enforced)
- Network path to JWKS endpoint (HTTPS required but no cert pinning)

### 6.2 JWT Validation Steps

```
1. SIGNATURE VERIFICATION
   ├─ Parse JWT header to get key ID (kid)
   ├─ Fetch JWKS from IdP endpoint (online) or use stored keys (offline)
   ├─ Find matching key by kid
   └─ Verify signature using public key (RS256, ES256, etc.)

2. STANDARD CLAIMS VALIDATION
   ├─ exp (expiration): Token must not be expired
   ├─ nbf (not before): Token must be active
   ├─ iat (issued at): Token has valid issue time
   └─ iss (issuer): Must match configured IdP issuer

3. IdP-LEVEL CLAIM VALIDATION
   ├─ Fetch all claims where robot_id = 0
   └─ Each claim must exist in token with exact value match

4. ROBOT MATCHING
   ├─ Execute claim matching algorithm
   ├─ Select robot with most matching claims
   ├─ Verify robot is not disabled
   └─ Verify robot is not expired (expires_at check)
```

### 6.3 Online vs Offline Validation

| Aspect | Online Mode | Offline Mode |
|--------|-------------|--------------|
| **JWKS Source** | Fetched from `jwks_uri` on every request | Stored in `jwks_keys` DB column |
| **Key Rotation** | Automatic (IdP rotates, Harbor fetches new) | Manual (admin must update stored keys) |
| **Network Dependency** | Requires connectivity to IdP | None (air-gapped compatible) |
| **Latency** | Additional HTTP request per auth | Direct DB lookup |
| **Use Case** | Public IdPs (GitHub, GitLab, GCP) | Private clusters, air-gapped environments |
| **Configuration** | `offline_validation = false`, set `jwks_uri` | `offline_validation = true`, set `jwks_keys` |

**JWKS Caching Note:** In online mode, Harbor re-fetches JWKS from the IdP endpoint on **every authentication request**. There is no caching layer. This ensures fresh keys but adds latency.

### 6.4 Attack Vectors & Mitigations

| Attack | Description | Mitigation |
|--------|-------------|------------|
| **Token Replay** | Reusing a valid JWT | Short token lifetimes (minutes); `exp` validation |
| **Token Substitution** | Using a valid token from wrong IdP | Issuer validation against configured IdP |
| **Key Confusion** | Tricking Harbor into using wrong key | `kid` header must match JWKS key ID |
| **Claim Injection** | Modifying token claims | Cryptographic signature verification |
| **JWKS Endpoint Compromise** | Attacker serves malicious JWKS | HTTPS required; admin-configured endpoint only |
| **Expired Robot Access** | Using JWT after robot disabled/expired | Robot status checked after claim matching |

---

## 7. Integration Patterns

### 7.1 Kubernetes Integration

Harbor integrates with Kubernetes clusters using projected ServiceAccount tokens.

#### Prerequisites

1. Kubernetes 1.24+ with ServiceAccountTokenVolumeProjection enabled
2. `credential-provider-echo-token` installed on nodes
3. KubeletConfig configured for Harbor authentication

#### Setup Steps

**1. Configure kubelet credential provider:**

```yaml
# /etc/kubernetes/credential-provider-config.yaml
apiVersion: kubelet.config.k8s.io/v1
kind: CredentialProviderConfig
providers:
  - name: credential-provider-echo-token
    matchImages:
      - "harbor.example.com/*"
    defaultCacheDuration: "5m"
    apiVersion: credentialprovider.kubelet.k8s.io/v1
    args:
      - "--audience=harbor.example.com"
```

**2. Create Federated IdP in Harbor:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "k8s-cluster-prod",
    "issuer": "https://kubernetes.default.svc.cluster.local",
    "jwks_uri": "https://kubernetes.default.svc.cluster.local/openid/v1/jwks",
    "offline_validation": false,
    "project_id": 0
  }'
```

**3. Add IdP-level claims:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps/1/claims" \
  -H "Content-Type: application/json" \
  -d '[
    {"claim_path": "iss", "value": "https://kubernetes.default.svc.cluster.local"},
    {"claim_path": "aud", "value": "harbor.example.com"}
  ]'
```

**4. Create federated robot with claims:**

```bash
# Create robot account
curl -X POST "https://harbor.example.com/api/v2.0/robots" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "k8s-prod-deployer",
    "level": "system",
    "permissions": [...],
    "federated_idp_id": 1
  }'

# Add robot-specific claims
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps/1/claims" \
  -H "Content-Type: application/json" \
  -d '[
    {"claim_path": "kubernetes.io.namespace", "value": "production", "robot_id": 1}
  ]'
```

#### Sample Kubernetes JWT Payload

```json
{
  "aud": ["harbor.example.com"],
  "exp": 1702584000,
  "iat": 1702580400,
  "iss": "https://kubernetes.default.svc.cluster.local",
  "kubernetes.io": {
    "namespace": "production",
    "pod": {
      "name": "my-app-7d4f8b9c5-x2k9m",
      "uid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
    },
    "serviceaccount": {
      "name": "my-app-sa",
      "uid": "11111111-2222-3333-4444-555555555555"
    }
  },
  "nbf": 1702580400,
  "sub": "system:serviceaccount:production:my-app-sa"
}
```

### 7.2 GitHub Actions Integration

#### Prerequisites

1. GitHub repository with Actions enabled
2. Harbor accessible from GitHub Actions runners

#### Setup Steps

**1. Create Federated IdP in Harbor:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "github-actions",
    "issuer": "https://token.actions.githubusercontent.com",
    "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
    "offline_validation": false,
    "project_id": 0
  }'
```

**2. Add IdP-level and robot claims:**

```bash
# IdP-level claims (apply to all robots)
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps/2/claims" \
  -d '[
    {"claim_path": "iss", "value": "https://token.actions.githubusercontent.com"},
    {"claim_path": "aud", "value": "https://harbor.example.com"}
  ]'

# Robot-specific claims
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps/2/claims" \
  -d '[
    {"claim_path": "repository", "value": "myorg/myrepo", "robot_id": 2}
  ]'
```

**3. GitHub Actions Workflow:**

```yaml
name: Build and Push
on: [push]

permissions:
  id-token: write  # Required for OIDC token
  contents: read

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Get OIDC Token
        id: oidc
        run: |
          TOKEN=$(curl -sLS "${ACTIONS_ID_TOKEN_REQUEST_URL}&audience=https://harbor.example.com" \
            -H "Authorization: Bearer ${ACTIONS_ID_TOKEN_REQUEST_TOKEN}" \
            -H "Accept: application/json" | jq -r '.value')
          echo "token=${TOKEN}" >> $GITHUB_OUTPUT

      - name: Login to Harbor
        run: |
          echo "${{ steps.oidc.outputs.token }}" | \
            docker login harbor.example.com -u jwt --password-stdin

      - name: Build and Push
        run: |
          docker build -t harbor.example.com/myproject/myapp:${{ github.sha }} .
          docker push harbor.example.com/myproject/myapp:${{ github.sha }}
```

#### Sample GitHub Actions JWT Payload

```json
{
  "iss": "https://token.actions.githubusercontent.com",
  "sub": "repo:myorg/myrepo:ref:refs/heads/main",
  "aud": "https://harbor.example.com",
  "exp": 1702584000,
  "iat": 1702580400,
  "nbf": 1702580400,
  "jti": "example-id-12345",
  "ref": "refs/heads/main",
  "sha": "abc123def456",
  "repository": "myorg/myrepo",
  "repository_owner": "myorg",
  "repository_owner_id": "12345678",
  "run_id": "1234567890",
  "run_number": "42",
  "run_attempt": "1",
  "actor": "developer",
  "actor_id": "87654321",
  "workflow": "Build and Push",
  "event_name": "push",
  "ref_type": "branch",
  "job_workflow_ref": "myorg/myrepo/.github/workflows/build.yml@refs/heads/main"
}
```

### 7.3 GitLab CI Integration

#### Prerequisites

1. GitLab 15.7+ with ID tokens enabled
2. Harbor accessible from GitLab runners

#### Setup Steps

**1. Create Federated IdP in Harbor:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gitlab-ci",
    "issuer": "https://gitlab.example.com",
    "jwks_uri": "https://gitlab.example.com/oauth/discovery/keys",
    "offline_validation": false,
    "project_id": 0
  }'
```

**2. GitLab CI Configuration:**

```yaml
# .gitlab-ci.yml
build:
  stage: build
  id_tokens:
    HARBOR_TOKEN:
      aud: https://harbor.example.com
  script:
    - echo "${HARBOR_TOKEN}" | docker login harbor.example.com -u jwt --password-stdin
    - docker build -t harbor.example.com/myproject/myapp:${CI_COMMIT_SHA} .
    - docker push harbor.example.com/myproject/myapp:${CI_COMMIT_SHA}
```

#### Sample GitLab CI JWT Payload

```json
{
  "iss": "https://gitlab.example.com",
  "sub": "project_path:mygroup/myproject:ref_type:branch:ref:main",
  "aud": "https://harbor.example.com",
  "exp": 1702584000,
  "iat": 1702580400,
  "nbf": 1702580400,
  "jti": "gitlab-jwt-id-12345",
  "namespace_id": "123",
  "namespace_path": "mygroup",
  "project_id": "456",
  "project_path": "mygroup/myproject",
  "user_id": "789",
  "user_login": "developer",
  "user_email": "developer@example.com",
  "pipeline_id": "1000",
  "pipeline_source": "push",
  "job_id": "2000",
  "ref": "main",
  "ref_type": "branch",
  "ref_protected": "true",
  "environment": "production",
  "environment_protected": "true"
}
```

---

## 8. REST API Reference

### 8.1 Identity Provider Endpoints

#### Create Identity Provider

```http
POST /api/v2.0/federated-idps
```

**Request Body:**
```json
{
  "name": "github-actions",
  "description": "GitHub Actions OIDC provider",
  "issuer": "https://token.actions.githubusercontent.com",
  "openid_config_url": "https://token.actions.githubusercontent.com/.well-known/openid-configuration",
  "offline_validation": false,
  "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
  "project_id": 0
}
```

**Response:** `201 Created`
```json
{
  "id": 1,
  "name": "github-actions",
  "issuer": "https://token.actions.githubusercontent.com",
  "offline_validation": false,
  "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
  "project_id": 0,
  "creation_time": "2024-12-15T10:30:00Z"
}
```

**Status Codes:**
| Code | Description |
|------|-------------|
| 201 | Created successfully |
| 400 | Invalid request (validation error) |
| 401 | Unauthorized |
| 403 | Forbidden |
| 409 | Conflict (name or `(issuer, audience)` already exists) |

**curl Example:**
```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "github-actions",
    "issuer": "https://token.actions.githubusercontent.com",
    "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
    "offline_validation": false,
    "project_id": 0
  }'
```

---

#### List Identity Providers

```http
GET /api/v2.0/federated-idps
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | int | Page number (default: 1) |
| `page_size` | int | Items per page (default: 10, max: 100) |
| `project_id` | int | Filter by project (0 for system-level) |

**Response:** `200 OK`
```json
[
  {
    "id": 1,
    "name": "github-actions",
    "issuer": "https://token.actions.githubusercontent.com",
    "offline_validation": false,
    "project_id": 0,
    "creation_time": "2024-12-15T10:30:00Z"
  }
]
```

---

#### Get Identity Provider

```http
GET /api/v2.0/federated-idps/{id}
```

**Response:** `200 OK`
```json
{
  "id": 1,
  "name": "github-actions",
  "description": "GitHub Actions OIDC provider",
  "issuer": "https://token.actions.githubusercontent.com",
  "openid_config_url": "https://token.actions.githubusercontent.com/.well-known/openid-configuration",
  "offline_validation": false,
  "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
  "project_id": 0,
  "creation_time": "2024-12-15T10:30:00Z",
  "update_time": "2024-12-15T10:30:00Z"
}
```

---

#### Update Identity Provider

```http
PUT /api/v2.0/federated-idps/{id}
```

**Mutable Fields:**
- `description` (always)
- `jwks_keys` (offline mode only)

**Immutable Fields:**
- `name`
- `issuer`
- `offline_validation`
- `openid_config_url`
- `jwks_uri`

**Request Body:**
```json
{
  "description": "Updated description"
}
```

**Response:** `200 OK`

---

#### Delete Identity Provider

```http
DELETE /api/v2.0/federated-idps/{id}
```

**Response:** `200 OK`

**Note:** Cannot delete IdP with associated robots. Delete robots first.

---

### 8.2 Claim Rules Endpoints

The `{id}` path parameter is authoritative for claim-rule mutations. Request
rules contain only `claim_path`, `value`, and optional `robot_id`; Harbor stamps
created rows and scopes deletions with the path IdP ID. The
`identity_provider_id` field remains present in GET responses. A non-positive
path ID returns 400, while an unknown positive ID returns 404 before any
persistence is attempted.

#### Add Claims

```http
POST /api/v2.0/federated-idps/{id}/claims
```

**Request Body:**
```json
{
  "rules": [
    {
      "claim_path": "iss",
      "value": "https://token.actions.githubusercontent.com",
      "robot_id": 0
    },
    {
      "claim_path": "repository",
      "value": "myorg/myrepo",
      "robot_id": 1
    }
  ]
}
```

**Response:** `201 Created`

---

#### List Claims

```http
GET /api/v2.0/federated-idps/{id}/claims
```

**Query Parameters:**
| Parameter | Type | Description |
|-----------|------|-------------|
| `claim_path` | string | Filter by claim path |
| `robot_id` | int | Filter by robot ID (0 for IdP-level) |
| `idp_only` | bool | Return only IdP-level claims |

**Response:** `200 OK`
```json
[
  {
    "id": 1,
    "identity_provider_id": 1,
    "robot_id": 0,
    "claim_path": "iss",
    "value": "https://token.actions.githubusercontent.com"
  }
]
```

---

#### Delete Claims

```http
DELETE /api/v2.0/federated-idps/{id}/claims
```

**Request Body:**
```json
{
  "rules": [
    {
      "claim_path": "repository",
      "value": "myorg/myrepo",
      "robot_id": 1
    }
  ]
}
```

**Response:** `200 OK`

---

### 8.3 Utility Endpoints

#### Ping OpenID Configuration

```http
POST /api/v2.0/federated-idps/openid-config
```

**Request Body:**
```json
{
  "openid_config_url": "https://token.actions.githubusercontent.com/.well-known/openid-configuration"
}
```

**Response:** `200 OK`
```json
{
  "issuer": "https://token.actions.githubusercontent.com",
  "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
  "claims_supported": ["sub", "aud", "exp", "iat", "iss", "jti", "nbf", ...]
}
```

---

#### Ping JWKS

```http
POST /api/v2.0/federated-idps/jwks
```

**Request Body:**
```json
{
  "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks"
}
```

**Response:** `200 OK`
```json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "key-id-123",
      "n": "...",
      "e": "AQAB",
      "use": "sig",
      "alg": "RS256"
    }
  ]
}
```

---

## 9. Robot Account Integration

### 9.1 Claim Hierarchy

**Critical Concept:** Claims are organized in a two-level hierarchy:

```
┌─────────────────────────────────────────────────────────────┐
│                    IDENTITY PROVIDER                        │
│                                                             │
│  IdP-Level Claims (robot_id = 0)                           │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  iss = "https://token.actions.githubusercontent.com" │   │
│  │  aud = "https://harbor.example.com"                  │   │
│  └─────────────────────────────────────────────────────┘   │
│                         ▲                                   │
│                         │ MANDATORY BASE                    │
│                         │ (ALL tokens must satisfy)         │
│                         │                                   │
│  ┌──────────────────────┴──────────────────────────────┐   │
│  │                      ROBOTS                          │   │
│  │                                                      │   │
│  │  ┌─────────────────────┐  ┌─────────────────────┐   │   │
│  │  │  Robot A            │  │  Robot B            │   │   │
│  │  │  repository=myorg/  │  │  repository=myorg/  │   │   │
│  │  │    myrepo           │  │    otherrepo        │   │   │
│  │  │  ref=refs/heads/    │  │                     │   │   │
│  │  │    main             │  │                     │   │   │
│  │  └─────────────────────┘  └─────────────────────┘   │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Validation Order:**
1. IdP-level claims are validated FIRST (mandatory filter)
2. Robot-level claims determine WHICH robot matches

**Inheritance Rules:**
- Robots automatically inherit IdP-level claims
- Robots CANNOT override IdP-level claim paths
- Attempting to add a claim with the same path as an IdP claim returns an error

### 9.2 Claim Set Uniqueness

**Global Constraint:** Robot claim sets must be **globally unique** across ALL robots, even across different projects.

**Why?** Ensures deterministic matching - no two robots can have identical claim patterns.

**Enforcement:**
- Checked at robot creation time
- Returns `409 Conflict` if duplicate claim set exists

**Example Error:**
```json
{
  "errors": [{
    "code": "CONFLICT",
    "message": "robot claims exactly match existing robot (ID: 5). At least one claim must be different to uniquely identify this robot."
  }]
}
```

**How to Resolve:**
- Add additional differentiating claims to the new robot
- Use different claim values (e.g., different branch, environment)

### 9.3 Creating Federated Robots

**UI Workflow:**

1. Navigate to **Administration > Robot Accounts** (system) or **Project > Robot Accounts** (project)
2. Click **New Robot Account**
3. Select **Federated** authentication type
4. Choose Identity Provider from dropdown
5. Add robot-specific claims (must differ from other robots)
6. Assign permissions
7. Click **Create**

**API Workflow:**

```bash
# 1. Create robot with federated IdP
curl -X POST "https://harbor.example.com/api/v2.0/robots" \
  -d '{
    "name": "my-federated-robot",
    "level": "project",
    "permissions": [...],
    "federated_idp_id": 1
  }'

# 2. Add robot-specific claims
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps/1/claims" \
  -d '[
    {"claim_path": "repository", "value": "myorg/myrepo", "robot_id": 5},
    {"claim_path": "ref", "value": "refs/heads/main", "robot_id": 5}
  ]'
```

---

## 10. Quirks & Gotchas

### 10.1 Validation Rules

| Field | Constraint |
|-------|------------|
| **Name** | 1-64 characters, lowercase alphanumeric + `.-_` |
| **Issuer** | HTTPS URL only, immutable; may be shared across scopes |
| **Claim Path** | Max 128 characters |
| **Claim Value** | Max 256 characters |

**Issuer and Audience Routing:**
- IdPs may share an issuer across system and project scopes
- One non-empty IdP-level `aud` is required before an IdP can authenticate
- The exact `(issuer, audience)` pair is globally unique, case-sensitive after configured audience whitespace is trimmed
- `aud` and `iss` are immutable IdP-level identity claims and cannot be configured for robots

### 10.2 Common Mistakes

1. **Forgetting Required Claims**
   - Always configure `iss` and `aud` as IdP-level claims
   - Without these, tokens may match unexpectedly

2. **Whitespace in Claim Values**
   - Claim values are compared with exact string match after leading/trailing whitespace is trimmed
   - Internal whitespace is preserved and must match: `"main"` ≠ `"ma in"`

3. **Modifying Immutable Fields**
   - `name`, `issuer`, `offline_validation`, `jwks_uri`, `openid_config_url` cannot be changed
   - Delete and recreate IdP if these need to change

4. **Robot Claims Overlapping IdP Claims**
   - Robot claims CANNOT use the same `claim_path` as IdP-level claims
   - This is a security feature to prevent claim override attacks
   - Error: `"claim_path 'iss' already owned by identity provider"`

5. **Basic Auth Format for Docker**
   - Username must be `jwt` (literal string)
   - Password is the JWT token
   - Example: `echo "$JWT_TOKEN" | docker login -u jwt --password-stdin harbor.example.com`
   - Avoid `-p` (puts the token in shell history and process args)

### 10.3 Limitations

| Limitation | Description | Future Possibility |
|------------|-------------|-------------------|
| **No Regex Matching** | Claim values must match exactly, no patterns | May be added in future |
| **Flat Claim Paths** | Keys can contain dots but not nested traversal | May add dot-notation |
| **Single Audience** | `aud` claim supports single string only | May support arrays |
| **No Claim Inheritance** | Project-level IdPs don't inherit from system | By design |
| **No JWKS Caching** | Online mode fetches JWKS on every request | May add caching |

### 10.4 Debugging Tips

**Decode JWT for Troubleshooting:**
```bash
# Using jwt-cli
jwt decode $TOKEN

# Using jq
echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | jq .

# Online: jwt.io (for non-sensitive tokens only)
```

**Harbor Log Messages to Look For:**
```
# Successful auth
"robot JWT auth successful for robot-name, request: GET /v2/project/repo/manifests/tag"

# No IdP found
"expected one IdP for issuer https://example.com and audience, found 0"

# JWKS fetch failed
"failed to get JWK set: failed to fetch JWKS from https://..."

# IdP claims failed
"IdP claims validation failed: required claim aud not in token"

# No matching robot
"no matching robot found: no robot matched the token claims"

# Robot disabled/expired
"robot my-robot is disabled"
"robot my-robot is expired"
```

**Common Error Codes:**

| Status | Meaning | Check |
|--------|---------|-------|
| 401 | No valid credentials | Token format, signature |
| 403 | Authenticated but not authorized | Robot permissions, project access |
| 404 | Resource not found | Repository exists, tag exists |

---

## 11. Configuration Options

### System Configuration

| Setting | Description | Default |
|---------|-------------|---------|
| `enable_project_federated_idp` | Allow project-level IdP creation | `false` |

**Enable via API:**
```bash
curl -X PUT "https://harbor.example.com/api/v2.0/configurations" \
  -d '{"enable_project_federated_idp": true}'
```

**Enable via `harbor.yml`:**
```yaml
# Not directly configurable in harbor.yml
# Use API or UI after deployment
```

### System-Level vs Project-Level IdPs

| Aspect | System-Level | Project-Level |
|--------|--------------|---------------|
| **Scope** | All projects | Single project |
| **Creation** | System admin only | Project admin (if enabled) |
| **`project_id`** | 0 | Project ID |
| **Robot Association** | Any robot | Project robots only |

---

## 12. Database Schema Reference

### Full Migration Script

```sql
-- Table: identity_providers
CREATE TABLE IF NOT EXISTS identity_providers (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    issuer TEXT NOT NULL,
    openid_config_url TEXT,
    offline_validation BOOLEAN NOT NULL DEFAULT FALSE,
    supported_algorithms TEXT,
    claims_supported TEXT,
    jwks_uri TEXT,
    jwks_keys JSONB,
    project_id INT NOT NULL,
    creation_time TIMESTAMP DEFAULT NOW(),
    update_time TIMESTAMP DEFAULT NOW(),
    UNIQUE (issuer, project_id)
);

-- Table: robot_identity_providers
CREATE TABLE IF NOT EXISTS robot_identity_providers (
    id SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id INT NOT NULL REFERENCES robot(id) ON DELETE CASCADE,
    creation_time TIMESTAMP DEFAULT NOW(),
    UNIQUE (identity_provider_id, robot_id)
);

-- Table: claim_rules
CREATE TABLE IF NOT EXISTS claim_rules (
    id SERIAL PRIMARY KEY,
    identity_provider_id INT NOT NULL REFERENCES identity_providers(id) ON DELETE CASCADE,
    robot_id INT NOT NULL,
    claim_path TEXT NOT NULL,
    value TEXT,
    creation_time TIMESTAMP DEFAULT NOW()
);

-- Index for claim matching queries
CREATE INDEX idx_claim_rules_lookup
ON claim_rules (identity_provider_id, claim_path, value, robot_id);
```

### Index Explanation

`idx_claim_rules_lookup`: Composite index optimizing the claim matching query:
- Filters by `identity_provider_id` first (most selective)
- Then by `claim_path` and `value` for condition matching
- Finally `robot_id` for grouping

### Cascade Delete Behavior

| Parent | Child | On Delete |
|--------|-------|-----------|
| `identity_providers` | `robot_identity_providers` | CASCADE |
| `identity_providers` | `claim_rules` | CASCADE |
| `robot` | `robot_identity_providers` | CASCADE |

**Note:** Deleting an IdP cascades to `robot_identity_providers` and `claim_rules`, but NOT to `robot` table. Robots remain but lose their IdP association.

---

## 13. Code Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              API Layer                                       │
│  src/server/v2.0/handler/federated_idp.go                                   │
│  - HTTP request handling                                                     │
│  - Request validation (validateJWKSKeys, etc.)                              │
│  - Response formatting                                                       │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                           Controller Layer                                   │
│  src/controller/federatedidp/controller.go                                  │
│  - Business logic                                                            │
│  - Uniqueness validation                                                     │
│  - Cross-entity coordination                                                 │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                            Manager Layer                                     │
│  src/pkg/federatedidp/manager.go                                            │
│  - Thin wrapper over DAO                                                     │
│  - Transaction coordination                                                  │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                              DAO Layer                                       │
│  src/pkg/federatedidp/dao/dao.go                                            │
│  - Database operations                                                       │
│  - FindMatchingRobot algorithm                                              │
│  - Claim validation                                                          │
└─────────────────────────────────┬───────────────────────────────────────────┘
                                  │
┌─────────────────────────────────▼───────────────────────────────────────────┐
│                            Model Layer                                       │
│  src/pkg/federatedidp/model/model.go                                        │
│  - Data structures                                                           │
│  - ORM mappings                                                              │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                        Security Middleware                                   │
│  src/server/middleware/security/robotjwt.go                                 │
│  - JWT extraction (Bearer, Basic Auth)                                       │
│  - Token validation                                                          │
│  - Robot matching                                                            │
│  - Security context creation                                                 │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key File Locations

| File | Purpose |
|------|---------|
| `src/server/v2.0/handler/federated_idp.go` | REST API handlers |
| `src/controller/federatedidp/controller.go` | Business logic |
| `src/pkg/federatedidp/dao/dao.go` | Database operations, claim matching |
| `src/pkg/federatedidp/model/model.go` | Data models |
| `src/server/middleware/security/robotjwt.go` | JWT auth middleware |
| `make/migrations/postgresql/0172_8gears_identity_providers.up.sql` | DB schema |
| `api/v2.0/swagger.yaml` | OpenAPI specification |

---

## 14. Appendices

### A. Sample JWT Payloads

#### Kubernetes ServiceAccount Token

```json
{
  "aud": ["harbor.example.com"],
  "exp": 1702584000,
  "iat": 1702580400,
  "iss": "https://kubernetes.default.svc.cluster.local",
  "kubernetes.io": {
    "namespace": "production",
    "pod": {
      "name": "my-app-7d4f8b9c5-x2k9m",
      "uid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
    },
    "serviceaccount": {
      "name": "my-app-sa",
      "uid": "11111111-2222-3333-4444-555555555555"
    }
  },
  "nbf": 1702580400,
  "sub": "system:serviceaccount:production:my-app-sa"
}
```

**Typical Claim Mappings:**
| Claim Path | Example Value | Use Case |
|------------|---------------|----------|
| `kubernetes.io.namespace` | `production` | Environment isolation |
| `sub` | `system:serviceaccount:production:my-app-sa` | Specific service account |
| `kubernetes.io.serviceaccount.name` | `my-app-sa` | Application identity |

---

#### GitHub Actions OIDC Token

```json
{
  "iss": "https://token.actions.githubusercontent.com",
  "sub": "repo:myorg/myrepo:ref:refs/heads/main",
  "aud": "https://harbor.example.com",
  "exp": 1702584000,
  "iat": 1702580400,
  "nbf": 1702580400,
  "jti": "example-id-12345",
  "ref": "refs/heads/main",
  "sha": "abc123def456789012345678901234567890abcd",
  "repository": "myorg/myrepo",
  "repository_owner": "myorg",
  "repository_owner_id": "12345678",
  "run_id": "1234567890",
  "run_number": "42",
  "run_attempt": "1",
  "actor": "developer",
  "actor_id": "87654321",
  "workflow": "Build and Push",
  "event_name": "push",
  "ref_type": "branch",
  "job_workflow_ref": "myorg/myrepo/.github/workflows/build.yml@refs/heads/main"
}
```

**Typical Claim Mappings:**
| Claim Path | Example Value | Use Case |
|------------|---------------|----------|
| `repository` | `myorg/myrepo` | Repository-specific access |
| `ref` | `refs/heads/main` | Branch protection |
| `environment` | `production` | Deployment environment |
| `actor` | `developer` | User tracking (audit) |

---

#### GitLab CI OIDC Token

```json
{
  "iss": "https://gitlab.example.com",
  "sub": "project_path:mygroup/myproject:ref_type:branch:ref:main",
  "aud": "https://harbor.example.com",
  "exp": 1702584000,
  "iat": 1702580400,
  "nbf": 1702580400,
  "jti": "gitlab-jwt-id-12345",
  "namespace_id": "123",
  "namespace_path": "mygroup",
  "project_id": "456",
  "project_path": "mygroup/myproject",
  "user_id": "789",
  "user_login": "developer",
  "user_email": "developer@example.com",
  "pipeline_id": "1000",
  "pipeline_source": "push",
  "job_id": "2000",
  "ref": "main",
  "ref_type": "branch",
  "ref_protected": "true",
  "environment": "production",
  "environment_protected": "true"
}
```

**Typical Claim Mappings:**
| Claim Path | Example Value | Use Case |
|------------|---------------|----------|
| `project_path` | `mygroup/myproject` | Project-specific access |
| `ref` | `main` | Branch protection |
| `environment` | `production` | Deployment environment |
| `ref_protected` | `true` | Protected branch enforcement |

---

### B. OpenID Configuration Examples

#### Kubernetes API Server

```
GET https://kubernetes.default.svc.cluster.local/.well-known/openid-configuration
```

```json
{
  "issuer": "https://kubernetes.default.svc.cluster.local",
  "jwks_uri": "https://kubernetes.default.svc.cluster.local/openid/v1/jwks",
  "response_types_supported": ["id_token"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"]
}
```

#### GitHub Actions

```
GET https://token.actions.githubusercontent.com/.well-known/openid-configuration
```

```json
{
  "issuer": "https://token.actions.githubusercontent.com",
  "jwks_uri": "https://token.actions.githubusercontent.com/.well-known/jwks",
  "subject_types_supported": ["public", "pairwise"],
  "response_types_supported": ["id_token"],
  "claims_supported": [
    "sub", "aud", "exp", "iat", "iss", "jti", "nbf",
    "ref", "sha", "repository", "repository_owner", "actor",
    "workflow", "event_name", "ref_type", "environment"
  ],
  "id_token_signing_alg_values_supported": ["RS256"],
  "scopes_supported": ["openid"]
}
```

#### GitLab

```
GET https://gitlab.example.com/.well-known/openid-configuration
```

```json
{
  "issuer": "https://gitlab.example.com",
  "authorization_endpoint": "https://gitlab.example.com/oauth/authorize",
  "token_endpoint": "https://gitlab.example.com/oauth/token",
  "jwks_uri": "https://gitlab.example.com/oauth/discovery/keys",
  "response_types_supported": ["code", "token", "id_token"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"],
  "claims_supported": [
    "iss", "sub", "aud", "exp", "iat", "nbf", "jti",
    "namespace_id", "namespace_path", "project_id", "project_path",
    "user_id", "user_login", "user_email",
    "pipeline_id", "job_id", "ref", "ref_type", "environment"
  ]
}
```

---

### C. JWKS Examples

#### RSA Key (RS256)

```json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "key-id-rsa-001",
      "use": "sig",
      "alg": "RS256",
      "n": "0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw",
      "e": "AQAB"
    }
  ]
}
```

#### ECDSA Key (ES256)

```json
{
  "keys": [
    {
      "kty": "EC",
      "kid": "key-id-ec-001",
      "use": "sig",
      "alg": "ES256",
      "crv": "P-256",
      "x": "f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU",
      "y": "x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0"
    }
  ]
}
```

**Required Fields for Validation:**
| Field | Description | Required |
|-------|-------------|----------|
| `kty` | Key type (RSA, EC) | Yes |
| `kid` | Key ID (matches JWT header) | Yes |
| `use` | Key usage (sig = signature) | No (assumed sig) |
| `alg` | Algorithm (RS256, ES256) | No (inferred from kty) |
| `n`, `e` | RSA modulus and exponent | Yes (RSA) |
| `x`, `y`, `crv` | EC curve parameters | Yes (EC) |

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2024-12-15 | Harbor Community | Initial release |

---

*This document is maintained in the Harbor repository at `docs/ADR-federated-idp.md`*
