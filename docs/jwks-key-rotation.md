# JWKS Key Rotation Guide for Harbor Federated Identity

This guide explains how Harbor manages JSON Web Key Sets (JWKS) for federated identity providers, including automatic key rotation in Online mode and manual key management in Offline mode.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Online Mode (Automatic Key Rotation)](#online-mode-automatic-key-rotation)
- [Offline Mode (Manual Key Rotation)](#offline-mode-manual-key-rotation)
- [Configuration Reference](#configuration-reference)
- [Integration Examples](#integration-examples)
- [Troubleshooting](#troubleshooting)
- [Security Considerations](#security-considerations)

---

## Overview

### What is JWKS?

A JSON Web Key Set (JWKS) is a set of cryptographic keys used to verify JWT (JSON Web Token) signatures. When a robot account authenticates to Harbor using a JWT from an external identity provider (IdP), Harbor uses the IdP's public keys from the JWKS to verify the token's authenticity.

### Why Key Rotation Matters

Cryptographic keys should be rotated regularly to:
- Limit the impact of a compromised key
- Follow security best practices and compliance requirements
- Ensure continued trust in your authentication system

### Harbor's Two Validation Modes

Harbor supports two modes for JWKS management:

| Mode | Description | Key Rotation | Use Case |
|------|-------------|--------------|----------|
| **Online** | Harbor fetches JWKS from your IdP's endpoint | Automatic | Cloud IdPs, connected environments |
| **Offline** | Admin manually provides JWKS keys | Manual | Air-gapped environments, custom IdPs |

---

## Quick Start

### Decision Matrix

| Your Scenario | Recommended Mode | Why |
|---------------|------------------|-----|
| Using AWS Cognito, Okta, Keycloak, or similar | **Online** | Automatic key rotation with no maintenance |
| Air-gapped / disconnected environment | **Offline** | No outbound network access needed |
| Custom IdP without standard JWKS endpoint | **Offline** | Direct key management |
| Strict security requiring manual key approval | **Offline** | Full control over key changes |
| Testing or development | **Online** | Simpler setup |

### Creating an Online Mode IdP

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-cloud-idp",
    "issuer": "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_xxxxx",
    "jwks_uri": "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_xxxxx/.well-known/jwks.json",
    "offline_validation": false,
    "project_id": 0
  }'
```

### Creating an Offline Mode IdP

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-airgapped-idp",
    "issuer": "https://my-internal-idp.local",
    "offline_validation": true,
    "jwks_keys": "{\"keys\":[{\"kty\":\"RSA\",\"kid\":\"key-1\",\"n\":\"...\",\"e\":\"AQAB\"}]}",
    "project_id": 0
  }'
```

---

## Online Mode (Automatic Key Rotation)

Online mode is the recommended approach for most deployments. Harbor automatically fetches and caches JWKS from your IdP, handling key rotations transparently.

### How It Works

When you configure an IdP in Online mode, Harbor:

1. **On IdP Creation**: Immediately fetches JWKS from the `jwks_uri` endpoint
2. **On Token Validation**: Uses cached keys, refreshing when the cache expires
3. **On IdP Update**: Force-refreshes the cache (bypasses rate limiting)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Online Mode - Token Validation Flow                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Robot Client                Harbor Core                  External IdP  │
│       │                          │                             │        │
│       │  1. JWT Token            │                             │        │
│       │─────────────────────────>│                             │        │
│       │                          │                             │        │
│       │                    ┌─────┴─────┐                       │        │
│       │                    │ In-Memory │                       │        │
│       │                    │   Cache   │                       │        │
│       │                    │  (15 min) │                       │        │
│       │                    └─────┬─────┘                       │        │
│       │                          │                             │        │
│       │                    Cache hit? ──Yes──> Validate Token  │        │
│       │                          │                             │        │
│       │                         No                             │        │
│       │                          ▼                             │        │
│       │                    ┌─────┴─────┐                       │        │
│       │                    │    DB     │                       │        │
│       │                    │   Cache   │                       │        │
│       │                    │ (max-age) │                       │        │
│       │                    └─────┬─────┘                       │        │
│       │                          │                             │        │
│       │                    Cache valid? ──Yes──> Validate Token│        │
│       │                          │                             │        │
│       │                         No                             │        │
│       │                          ▼                             │        │
│       │                    Rate limited? ──Yes──> Use stale DB │        │
│       │                          │                      cache  │        │
│       │                         No                             │        │
│       │                          │  2. GET jwks_uri            │        │
│       │                          │────────────────────────────>│        │
│       │                          │                             │        │
│       │                          │  3. JWKS + Cache-Control    │        │
│       │                          │<────────────────────────────│        │
│       │                          │                             │        │
│       │                    Update both caches                  │        │
│       │                          │                             │        │
│       │                    Validate Token                      │        │
│       │                          │                             │        │
│       │  4. Success/Failure      │                             │        │
│       │<─────────────────────────│                             │        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Two-Tier Caching Architecture

Harbor uses a two-tier cache to optimize performance and ensure consistency across multiple Harbor instances:

**Tier 1: In-Memory Cache (Per-Process)**
- Fastest access for high-traffic scenarios
- Default TTL: 15 minutes
- Scope: Single Harbor instance
- Automatically synchronized with DB cache

**Tier 2: Database Cache (PostgreSQL)**
- Shared across all Harbor instances in a cluster
- TTL based on IdP's `Cache-Control: max-age` header (default: 1 hour)
- Serves as the source of truth

### Cache Behavior During Key Rotation

When your IdP rotates keys:

1. IdP publishes new keys to its JWKS endpoint (usually alongside old keys)
2. Harbor's cache eventually expires (based on `max-age`)
3. Next token validation triggers a cache refresh
4. New keys are fetched and cached
5. Tokens signed with new keys now validate successfully

**Grace Period**: Most IdPs maintain both old and new keys in their JWKS during rotation. This ensures tokens signed with either key validate successfully during the transition period.

### Triggering Immediate Cache Refresh

If you need to pick up key changes immediately (without waiting for cache expiry):

```bash
# Update any field on the IdP to force a cache refresh
curl -X PUT "https://harbor.example.com/api/v2.0/federated-idps/{id}" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Updated to refresh JWKS cache"
  }'
```

This bypasses rate limiting and immediately fetches fresh JWKS.

### Rate Limiting

To prevent overwhelming your IdP's JWKS endpoint, Harbor enforces rate limiting:

- **Default interval**: 5 minutes between fetch attempts
- **Behavior when rate-limited**: Returns stale cached keys (no timeout)
- **Bypass**: Rate limiting is bypassed on IdP create/update operations

---

## Offline Mode (Manual Key Rotation)

Offline mode is designed for environments where Harbor cannot reach external JWKS endpoints, such as air-gapped deployments or when using custom identity providers.

### How It Works

In Offline mode:
- Admin provides JWKS keys directly via the API
- No network calls are made to fetch keys
- Key rotation requires manual API updates

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Offline Mode - Token Validation Flow                 │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  Robot Client                Harbor Core                                │
│       │                          │                                      │
│       │  1. JWT Token            │                                      │
│       │─────────────────────────>│                                      │
│       │                          │                                      │
│       │                    ┌─────┴─────┐                                │
│       │                    │    DB     │                                │
│       │                    │ jwks_keys │                                │
│       │                    │  (JSONB)  │                                │
│       │                    └─────┬─────┘                                │
│       │                          │                                      │
│       │                    Parse JWKS JSON                              │
│       │                          │                                      │
│       │                    Find key by 'kid'                            │
│       │                          │                                      │
│       │                    Validate signature                           │
│       │                          │                                      │
│       │  2. Success/Failure      │                                      │
│       │<─────────────────────────│                                      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Manual Key Rotation Procedure

Follow these steps when your IdP rotates keys:

#### Step 1: Obtain New JWKS from Your IdP

Export the JWKS from your identity provider. The format should be:

```json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "new-key-2024",
      "use": "sig",
      "n": "0vx7agoebGcQSuuPiLJXZpt...",
      "e": "AQAB"
    },
    {
      "kty": "RSA",
      "kid": "old-key-2023",
      "use": "sig",
      "n": "ofgWCuLjybRlzo0tZWJjNiuSfb4p4fAkd...",
      "e": "AQAB"
    }
  ]
}
```

**Best Practice**: Include both old and new keys during the rotation period to allow existing tokens to continue validating.

#### Step 2: Validate JWKS Structure

Before updating Harbor, verify the JWKS is valid:

```bash
# Validate JWKS JSON structure
echo '{"keys":[...]}' | jq .

# Verify required fields are present for each key
echo '{"keys":[...]}' | jq '.keys[] | {kid, kty, n, e}'
```

Required fields per key:
- `kid`: Key ID (must match the `kid` in JWT headers)
- `kty`: Key type (e.g., "RSA", "EC")
- `n`, `e`: RSA modulus and exponent (for RSA keys)
- Or `x`, `y`, `crv`: Curve parameters (for EC keys)

#### Step 3: Update IdP with New JWKS

```bash
# Get current IdP ID
IDP_ID=$(curl -s "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" | \
  jq '.[] | select(.name=="my-airgapped-idp") | .id')

# Update with new JWKS
curl -X PUT "https://harbor.example.com/api/v2.0/federated-idps/${IDP_ID}" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "jwks_keys": "{\"keys\":[{\"kty\":\"RSA\",\"kid\":\"new-key-2024\",\"n\":\"...\",\"e\":\"AQAB\"},{\"kty\":\"RSA\",\"kid\":\"old-key-2023\",\"n\":\"...\",\"e\":\"AQAB\"}]}"
  }'
```

#### Step 4: Verify Token Validation

Test that tokens signed with the new key validate successfully:

```bash
# Test authentication with a JWT signed by the new key
curl -H "Authorization: Bearer ${JWT_TOKEN}" \
  "https://harbor.example.com/api/v2.0/projects"
```

#### Step 5: Remove Old Keys (After Transition Period)

Once all tokens signed with old keys have expired, remove the old keys:

```bash
curl -X PUT "https://harbor.example.com/api/v2.0/federated-idps/${IDP_ID}" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "jwks_keys": "{\"keys\":[{\"kty\":\"RSA\",\"kid\":\"new-key-2024\",\"n\":\"...\",\"e\":\"AQAB\"}]}"
  }'
```

### Best Practices for Manual Rotation

1. **Maintain key overlap**: Include both old and new keys during rotation
2. **Schedule during low traffic**: Perform rotations during maintenance windows
3. **Document rotation history**: Track when keys were rotated and by whom
4. **Test before production**: Validate tokens in a staging environment first
5. **Automate when possible**: Script the rotation process for consistency

---

## Configuration Reference

### Environment Variables

Configure JWKS caching behavior via environment variables on the Harbor Core service:

| Variable | Default | Description |
|----------|---------|-------------|
| `JWKS_CACHE_MIN_FETCH_INTERVAL` | `5m` | Minimum time between remote JWKS fetches (rate limiting) |
| `JWKS_CACHE_DEFAULT_TTL` | `1h` | Cache TTL when IdP doesn't provide `Cache-Control` header |
| `JWKS_CACHE_INMEMORY_TTL` | `15m` | In-memory cache TTL (per Harbor instance) |

**Example (docker-compose override):**

```yaml
services:
  core:
    environment:
      JWKS_CACHE_MIN_FETCH_INTERVAL: "10m"
      JWKS_CACHE_DEFAULT_TTL: "30m"
      JWKS_CACHE_INMEMORY_TTL: "5m"
```

**Value format**: Go duration strings (e.g., `5m`, `1h`, `30s`, `2h30m`)

### API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/v2.0/federated-idps` | GET | List all federated IdPs |
| `/api/v2.0/federated-idps` | POST | Create a new IdP |
| `/api/v2.0/federated-idps/{id}` | GET | Get IdP by ID |
| `/api/v2.0/federated-idps/{id}` | PUT | Update IdP (triggers JWKS refresh for online mode) |
| `/api/v2.0/federated-idps/{id}` | DELETE | Delete IdP |
| `/api/v2.0/federated-idps/jwks` | POST | Test-fetch JWKS from a URI |

### Database Schema

JWKS data is stored in the `identity_providers` table:

| Column | Type | Description |
|--------|------|-------------|
| `jwks_uri` | TEXT | URL to fetch JWKS (Online mode) |
| `jwks_keys` | JSONB | Stored JWKS JSON (both modes) |
| `jwks_cached_at` | TIMESTAMP | When JWKS was last cached (Online mode) |
| `jwks_expires_at` | TIMESTAMP | When cache expires (Online mode) |
| `jwks_last_fetch_attempt` | TIMESTAMP | Last fetch attempt for rate limiting |
| `offline_validation` | BOOLEAN | `true` = Offline mode, `false` = Online mode |

---

## Integration Examples

### AWS Cognito

AWS Cognito User Pools provide a standard JWKS endpoint.

**JWKS URI Format:**
```
https://cognito-idp.{region}.amazonaws.com/{userPoolId}/.well-known/jwks.json
```

**Example Configuration:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "aws-cognito-prod",
    "issuer": "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_AbCdEfGhI",
    "jwks_uri": "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_AbCdEfGhI/.well-known/jwks.json",
    "openid_config_url": "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_AbCdEfGhI/.well-known/openid-configuration",
    "offline_validation": false,
    "supported_algorithms": "RS256",
    "project_id": 0
  }'
```

**Key Rotation**: AWS Cognito automatically rotates keys. Harbor will pick up new keys when the cache expires (typically respecting Cognito's `Cache-Control` headers).

### Keycloak

Keycloak provides JWKS via its realm certificates endpoint.

**JWKS URI Format:**
```
https://{keycloak-host}/realms/{realm}/protocol/openid-connect/certs
```

**Example Configuration:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "keycloak-production",
    "issuer": "https://keycloak.example.com/realms/harbor",
    "jwks_uri": "https://keycloak.example.com/realms/harbor/protocol/openid-connect/certs",
    "openid_config_url": "https://keycloak.example.com/realms/harbor/.well-known/openid-configuration",
    "offline_validation": false,
    "supported_algorithms": "RS256,ES256",
    "project_id": 0
  }'
```

**Key Rotation in Keycloak**:
1. Go to Realm Settings > Keys
2. Add new key provider
3. Set priority higher than existing key
4. Old keys remain available during grace period

### Okta

Okta provides JWKS via its authorization server keys endpoint.

**JWKS URI Format:**
```
https://{okta-domain}/oauth2/{authServerId}/v1/keys
```

For the default authorization server:
```
https://{okta-domain}/oauth2/default/v1/keys
```

**Example Configuration:**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "okta-production",
    "issuer": "https://dev-123456.okta.com/oauth2/default",
    "jwks_uri": "https://dev-123456.okta.com/oauth2/default/v1/keys",
    "openid_config_url": "https://dev-123456.okta.com/oauth2/default/.well-known/openid-configuration",
    "offline_validation": false,
    "supported_algorithms": "RS256",
    "project_id": 0
  }'
```

**Key Rotation**: Okta rotates keys approximately 4 times per year. New keys are published several weeks before rotation to allow downstream caching to update.

### Custom IdP (Offline Mode)

For custom identity providers or air-gapped environments.

**Step 1: Generate RSA Key Pair (if needed)**

```bash
# Generate private key
openssl genrsa -out private.pem 2048

# Extract public key
openssl rsa -in private.pem -pubout -out public.pem

# Convert to JWK format (using a tool like step-cli or node)
step crypto jwk create --from-pem public.pem --kid "my-key-2024" > jwk.json
```

**Step 2: Create JWKS**

```json
{
  "keys": [
    {
      "kty": "RSA",
      "kid": "my-key-2024",
      "use": "sig",
      "alg": "RS256",
      "n": "0vx7agoebGcQSuuPiLJXZptN9nndrQmbXEps2aiAFbWhM78LhWx4cbbfAAtVT86zwu1RK7aPFFxuhDR1L6tSoc_BJECPebWKRXjBZCiFV4n3oknjhMstn64tZ_2W-5JsGY4Hc5n9yBXArwl93lqt7_RN5w6Cf0h4QyQ5v-65YGjQR0_FDW2QvzqY368QQMicAtaSqzs8KJZgnYb9c7d0zgdAZHzu6qMQvRL5hajrn1n91CbOpbISD08qNLyrdkt-bFTWhAI4vMQFh6WeZu0fM4lFd2NcRwr3XPksINHaQ-G_xBniIqbw0Ls1jF44-csFCur-kEgU8awapJzKnqDKgw",
      "e": "AQAB"
    }
  ]
}
```

**Step 3: Configure Harbor IdP**

```bash
# Escape the JSON for embedding
JWKS_ESCAPED=$(cat jwks.json | jq -c . | sed 's/"/\\"/g')

curl -X POST "https://harbor.example.com/api/v2.0/federated-idps" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"custom-airgapped-idp\",
    \"issuer\": \"https://internal-idp.corp.local\",
    \"offline_validation\": true,
    \"jwks_keys\": \"${JWKS_ESCAPED}\",
    \"supported_algorithms\": \"RS256\",
    \"project_id\": 0
  }"
```

---

## Troubleshooting

### Common Issues

| Symptom | Likely Cause | Solution |
|---------|-------------|----------|
| `key not found in JWKS` | JWT `kid` doesn't match any key in JWKS | Verify the JWT's `kid` header matches a key in your IdP's JWKS |
| `failed to fetch JWKS` | Network issue or invalid `jwks_uri` | Check Harbor Core logs; verify URL is accessible from Harbor |
| `IdP has no JWKS keys configured` | Offline IdP missing `jwks_keys` | Update IdP with valid JWKS JSON |
| `JWKS fetch rate limited` | Too many validation attempts | Wait 5 minutes or increase `JWKS_CACHE_MIN_FETCH_INTERVAL` |
| `JWT signature validation failed` | Key mismatch or expired cache | Update IdP to force refresh (Online) or update `jwks_keys` (Offline) |
| `no robot matched the token claims` | Robot claims don't match JWT | Verify claim rules match the JWT's actual claims |

### Debugging Steps

**1. Check Harbor Core Logs**

```bash
# Docker Compose
docker compose logs core | grep -i jwks

# Kubernetes
kubectl logs -l app=harbor,component=core | grep -i jwks
```

Look for messages like:
- `JWKS cache hit (in-memory)` - Using cached keys
- `JWKS cache hit (DB)` - Using database-cached keys
- `JWKS cache miss, fetching from` - Fetching fresh keys
- `JWKS rate limited` - Rate limiting in effect
- `Failed to cache JWKS` - Fetch errors

**2. Verify IdP JWKS Endpoint (Online Mode)**

```bash
# Test JWKS endpoint accessibility
curl -v "https://your-idp.example.com/.well-known/jwks.json"

# Check Cache-Control headers
curl -I "https://your-idp.example.com/.well-known/jwks.json" | grep -i cache
```

**3. Validate JWT Structure**

```bash
# Base64URL decoder (JWT segments use RFC 7515 base64url, not standard base64)
b64url_decode() {
  local s="${1//-/+}"; s="${s//_//}"
  local pad=$(( (4 - ${#s} % 4) % 4 ))
  while (( pad-- > 0 )); do s+="="; done
  printf '%s' "$s" | base64 -d
}

# Decode JWT header (without verification)
b64url_decode "$(echo YOUR_JWT_TOKEN | cut -d. -f1)" | jq .

# Expected output should include:
# {
#   "alg": "RS256",
#   "kid": "key-id-here",
#   "typ": "JWT"
# }
```

**4. Verify Key ID Match**

```bash
# Get JWT kid
JWT_KID=$(b64url_decode "$(echo YOUR_JWT_TOKEN | cut -d. -f1)" | jq -r '.kid')

# Get JWKS kids
curl -s "https://your-idp.example.com/.well-known/jwks.json" | jq '.keys[].kid'

# Verify match
echo "JWT kid: $JWT_KID"
```

**5. Test JWKS Fetch via Harbor API**

```bash
curl -X POST "https://harbor.example.com/api/v2.0/federated-idps/jwks" \
  -H "Authorization: Basic $(echo -n 'admin:Harbor12345' | base64)" \
  -H "Content-Type: application/json" \
  -d '{
    "jwks_uri": "https://your-idp.example.com/.well-known/jwks.json"
  }'
```

---

## Security Considerations

### Always Use HTTPS

JWKS endpoints must be served over HTTPS. Harbor validates URLs and will reject `http://` URIs for `jwks_uri`. This prevents:
- Man-in-the-middle attacks
- Key injection attacks
- Traffic interception

### Key ID (kid) Requirements

All JWTs used with Harbor federated identity must include a `kid` (Key ID) in the header:

```json
{
  "alg": "RS256",
  "kid": "unique-key-identifier",
  "typ": "JWT"
}
```

The `kid` is used to:
- Select the correct key from the JWKS
- Support key rotation with multiple active keys
- Prevent key confusion attacks

### Grace Periods During Rotation

When rotating keys:

1. **Publish new key first**: Add the new key to JWKS before using it to sign tokens
2. **Maintain overlap**: Keep old keys in JWKS until all tokens signed with them expire
3. **Typical overlap period**: 24 hours to 7 days, depending on token lifetime
4. **Remove old keys last**: Only remove old keys after the grace period

### Algorithm Restrictions

Configure `supported_algorithms` on your IdP to restrict which signing algorithms are accepted:

```json
{
  "supported_algorithms": "RS256,ES256"
}
```

This prevents algorithm confusion attacks where an attacker might try to use a weaker algorithm.

### Monitoring Recommendations

- Alert on repeated JWKS fetch failures
- Monitor cache hit/miss ratios
- Track authentication failures by IdP
- Review key rotation events in audit logs

---

## Additional Resources

- [RFC 7517 - JSON Web Key (JWK)](https://tools.ietf.org/html/rfc7517)
- [RFC 7519 - JSON Web Token (JWT)](https://tools.ietf.org/html/rfc7519)
- [OpenID Connect Discovery](https://openid.net/specs/openid-connect-discovery-1_0.html)
- [Harbor Federated Identity Documentation](https://goharbor.io/docs/)
