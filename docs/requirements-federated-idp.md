# Federated IdP Component Requirements

This document defines the validation rules, constraints, and behavior for the Federated Identity Provider (IdP) API.

## Table of Contents
- [Data Model Constraints](#data-model-constraints)
- [CREATE Route Requirements](#create-route-requirements)
- [UPDATE Route Requirements](#update-route-requirements)
- [DELETE Route Requirements](#delete-route-requirements)
- [LIST Route Requirements](#list-route-requirements)
- [GET Route Requirements](#get-route-requirements)
- [Validation Modes](#validation-modes)
- [Edge Cases](#edge-cases)

---

## Data Model Constraints

### Name Field
| Constraint | Value |
|------------|-------|
| Min Length | 1 |
| Max Length | 64 |
| Must Start With | Letter (a-z) |
| Allowed Characters | Lowercase letters (a-z), numbers (0-9), `.`, `-`, `_` |
| Case Sensitivity | Must be lowercase; uppercase returns error |
| Whitespace | Leading/trailing spaces trimmed; internal spaces NOT allowed |
| Uniqueness | Unique per project (project_id scope) |

### Issuer Field
| Constraint | Value |
|------------|-------|
| Required | Yes |
| Format | Valid HTTPS URL |
| Uniqueness | **Globally unique** across all projects |
| Mutability | **Immutable after creation** |
| Max Length | 2048 characters |

### Description Field
| Constraint | Value |
|------------|-------|
| Required | No (optional) |
| Min Length | 0 |
| Max Length | 264 |
| Mutability | Mutable (can be updated) |

### URL Fields (jwks_uri, openid_config_url)
| Constraint | Value |
|------------|-------|
| Protocol | HTTPS only (HTTP not allowed) |
| Max Length | 2048 characters |

### Supported Algorithms
| Constraint | Value |
|------------|-------|
| Format | Uppercase letters and numbers only |
| Min Length | 3 characters per algorithm |
| Max Length | 64 characters per algorithm |
| Example Valid | RS256, RS384, RS512, ES256, ES384, ES512, PS256 |

### Project ID
| Constraint | Value |
|------------|-------|
| Required | Yes |
| Value | 0 = system-level, >0 = project-level |
| Mutability | **Immutable after creation** |

---

## CREATE Route Requirements

### POST `/api/v2.0/federated-idps`

#### Online Validation Mode (`offline_validation: false`)

**Required Fields:**
- `name` - IdP name (validated per constraints above)
- `openid_config_url` - OpenID Connect discovery URL
- `jwks_uri` - JSON Web Key Set URI
- `project_id` - Project scope (0 for system)

**Derived Fields (from OpenID discovery):**
- `issuer` - Extracted from discovery document
- `claims_supported` - Optional, from discovery
- `supported_algorithms` - Optional, from discovery (`id_token_signing_alg_values_supported`)

**Optional Fields:**
- `description` - Human-readable description

**Ignored Fields (should not be provided):**
- `jwks_keys` - Not used in online mode

**Validation Steps:**
1. Trim whitespace from `name`
2. Validate `name` format (starts with letter, lowercase alphanumeric with `.-_`)
3. Check `name` global uniqueness
4. Validate `openid_config_url` is valid HTTPS URL
5. Validate `jwks_uri` is valid HTTPS URL
6. Fetch OpenID discovery document from `openid_config_url`
7. Extract and validate `issuer` from discovery
8. Verify `jwks_uri` matches discovery document's `jwks_uri`
9. Store the IdP; its audience is configured as a required IdP-level claim in the follow-up claim-rules call
10. Store derived `claims_supported` and `supported_algorithms`

#### Offline Validation Mode (`offline_validation: true`)

**Required Fields:**
- `name` - IdP name
- `issuer` - Token issuer URL (HTTPS)
- `jwks_keys` - JWKS JSON object with keys
- `project_id` - Project scope

**Optional Fields:**
- `description` - Human-readable description
- `supported_algorithms` - Manually specified algorithms
- `claims_supported` - Manually specified claims

**Ignored Fields (should not be provided):**
- `openid_config_url` - Not used in offline mode
- `jwks_uri` - Not used in offline mode

**Validation Steps:**
1. Trim whitespace from `name`
2. Validate `name` format
3. Check `name` global uniqueness
4. Validate `issuer` is valid HTTPS URL
5. Store the IdP; its audience is configured as a required IdP-level claim in the follow-up claim-rules call
6. Validate `jwks_keys` structure:
   - Must be valid JSON object
   - Must contain `keys` array
   - Each key must have required JWK fields (`kty`, `kid`, etc.)
7. If `supported_algorithms` provided, validate format (uppercase + numbers, 3-64 chars each)

---

## UPDATE Route Requirements

### PUT `/api/v2.0/federated-idps/{id}`

**Critical Restrictions:**
- **Cannot change `issuer`** - Immutable
- **Cannot change `project_id`** - Immutable
- **Cannot switch validation modes** - `offline_validation` is immutable
- **Cannot change `name`** - Immutable (breaks robot associations)

#### Online Mode Updates

**Allowed Fields:**
- `description` - Can be updated

**Disallowed Fields:**
- Everything else (issuer, jwks_uri, openid_config_url, etc.)

#### Offline Mode Updates

**Allowed Fields:**
- `description` - Can be updated
- `jwks_keys` - Can rotate/update keys

**Disallowed Fields:**
- Everything else

**Validation Steps:**
1. Fetch existing IdP by ID
2. Verify user has update permission
3. Check which fields are being modified
4. Reject if immutable fields are being changed
5. Validate allowed fields per mode
6. If `jwks_keys` updated (offline mode), validate structure
7. Apply update

---

## Identity Claim Routing

An IdP is selected by the exact, case-sensitive pair of its token `iss` claim and one token `aud` value. The issuer may therefore be reused across system and project IdPs, but each `(issuer, audience)` pair is globally unique. Audience values are trimmed when configured.

IdPs are created in two calls. Until exactly one non-empty IdP-level `aud` claim is added, an IdP is incomplete and cannot authenticate. Existing audience-less IdPs remain unusable until configured; Harbor does not backfill audiences.

The IdP-level `aud` and `iss` rules are identity claims: they cannot be added at robot level, deleted, or replaced after creation. `aud` and `iss` remain valid required claims even when discovery omits them from `claims_supported`.

---

## DELETE Route Requirements

### DELETE `/api/v2.0/federated-idps/{id}`

**Pre-conditions:**
1. IdP must exist
2. User must have delete permission
3. **No robots can be associated** - Must delete robots first

**Cascade Behavior:**
- Delete all `claim_rules` associated with the IdP (where `identity_provider_id = id`)
- Delete all `robot_identity_providers` records (should be 0 if pre-condition met)

**Error Cases:**
- 404: IdP not found
- 400: Robots still associated (message: "Please delete the associated robots before deleting the federated idp")
- 403: Insufficient permissions

---

## LIST Route Requirements

### GET `/api/v2.0/federated-idps`

**Query Parameters:**
- `q` - Query filter string
- `sort` - Sort field
- `page` - Page number
- `pageSize` - Items per page

**Supported Filters:**
- `Level=system` - System-level IdPs only (`project_id = 0`)
- `Level=project` - Project-level IdPs (requires `ProjectID`)
- `ProjectID=<id>` - Filter by specific project
- `name=<name>` - Filter by IdP name

**Default Behavior:**
- No Level specified → returns system-level IdPs

**Validation:**
- `Level` must be "system" or "project"
- `ProjectID` must be positive integer when `Level=project`
- `Level=project` requires `ProjectID` to be specified

---

## GET Route Requirements

### GET `/api/v2.0/federated-idps/{id}`

**Path Parameters:**
- `id` - IdP ID (int64)

**Response:**
- Full IdP object including all fields
- `jwks_keys` included for offline mode IdPs

**Error Cases:**
- 404: IdP not found
- 403: Insufficient permissions

---

## Validation Modes

### Online Validation (Default)

```
┌─────────────────────────────────────────────────────────────────┐
│                    Online Validation Flow                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Client Request                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ {                                                        │   │
│  │   "name": "my-idp",                                      │   │
│  │   "openid_config_url": "https://idp/.well-known/...",   │   │
│  │   "jwks_uri": "https://idp/jwks",                       │   │
│  │   "project_id": 0                                        │   │
│  │ }                                                        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Fetch OpenID Discovery Document                         │   │
│  │ GET https://idp/.well-known/openid-configuration        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Extract: issuer, jwks_uri, claims_supported, algorithms │   │
│  │ Validate: jwks_uri matches provided value               │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Store in DB with derived fields                         │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Offline Validation

```
┌─────────────────────────────────────────────────────────────────┐
│                   Offline Validation Flow                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Client Request                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ {                                                        │   │
│  │   "name": "my-idp",                                      │   │
│  │   "issuer": "https://my-issuer.com",                    │   │
│  │   "offline_validation": true,                            │   │
│  │   "jwks_keys": { "keys": [...] },                       │   │
│  │   "project_id": 0                                        │   │
│  │ }                                                        │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Validate JWKS Keys Structure                            │   │
│  │ - Has "keys" array                                      │   │
│  │ - Each key has kty, kid, etc.                          │   │
│  └─────────────────────────────────────────────────────────┘   │
│                              │                                  │
│                              ▼                                  │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │ Store directly (no network fetches)                     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Edge Cases

### Input Sanitization

| Input | Behavior |
|-------|----------|
| `"  my-idp  "` | Trimmed to `"my-idp"` |
| `"My-IDP"` | **Error**: Name must be lowercase |
| `"my idp"` | **Error**: Spaces not allowed |
| `"123-idp"` | **Error**: Must start with letter |
| `""` (empty) | **Error**: Name is required |
| `null` | **Error**: Name is required |

### URL Validation

| Input | Behavior |
|-------|----------|
| `"http://example.com"` | **Error**: Must be HTTPS |
| `"https://example.com"` | Valid |
| `"not-a-url"` | **Error**: Invalid URL format |
| Very long URL (>2048) | **Error**: URL too long |

### JWKS Keys Validation (Offline Mode)

| Input | Behavior |
|-------|----------|
| `{}` | **Error**: Missing keys array |
| `{ "keys": [] }` | **Error**: Keys array is empty |
| `{ "keys": [{}] }` | **Error**: Key missing required fields |
| `{ "keys": [{"kty": "RSA", "kid": "1", ...}] }` | Valid |
| Invalid JSON | **Error**: Invalid JSON structure |

### Concurrent Operations

- Name uniqueness check should be atomic
- `(issuer, audience)` uniqueness check should be atomic when an IdP-level audience is added
- Consider race conditions in the follow-up claim-rules call

---

## Error Messages

| Code | Message |
|------|---------|
| 400 | `"name is required"` |
| 400 | `"name must start with a letter"` |
| 400 | `"name must be lowercase"` |
| 400 | `"name contains invalid characters"` |
| 400 | `"name exceeds maximum length of 64 characters"` |
| 400 | `"issuer is required"` |
| 400 | `"issuer must be a valid HTTPS URL"` |
| 400 | `"issuer already exists"` |
| 400 | `"jwks_keys is required for offline validation"` |
| 400 | `"jwks_keys must contain a 'keys' array"` |
| 400 | `"openid_config_url is required for online validation"` |
| 400 | `"jwks_uri is required for online validation"` |
| 400 | `"URL must use HTTPS"` |
| 400 | `"URL exceeds maximum length"` |
| 400 | `"cannot modify issuer after creation"` |
| 400 | `"cannot modify project_id after creation"` |
| 400 | `"cannot switch validation mode after creation"` |
| 400 | `"Please delete the associated robots before deleting the federated idp"` |
| 404 | `"federatedidp {id} not found"` |
| 409 | `"federatedidp with this name already exists"` |

---

---

## Claims API Requirements

### Data Model Constraints

#### ClaimRule Fields
| Field | Constraint |
|-------|------------|
| `claim_path` | Required, max 128 characters |
| `value` | Required, max 256 characters |
| `robot_id` | Optional, 0 = IdP-level claim, >0 = robot-specific claim |

`identity_provider_id` is response-only. For create and delete requests, the
`{id}` path parameter is the sole IdP scope and clients must not send an IdP ID
inside individual rules.

### LIST Claims Route

#### GET `/api/v2.0/federated-idps/{id}/claims`

**Query Parameters:**
- `claim_path` - Filter by specific claim path
- `robot_id` - Filter by robot ID (0 = IdP-level, >0 = specific robot)
- `idp_only` - If true, return only IdP-level claims (robot_id=0)

**Filtering Modes:**
| Parameters | Behavior |
|------------|----------|
| None | Returns all claims for the IdP |
| `idp_only=true` | Returns only IdP-level claims (robot_id=0) |
| `robot_id=0` | Returns only IdP-level claims |
| `robot_id=N` (N>0) | Returns only claims for robot N |
| `claim_path=X` | Filters by claim path (can combine with above) |

**Validation:**
- Cannot specify both `idp_only` and `robot_id` simultaneously

### CREATE Claims Route

#### POST `/api/v2.0/federated-idps/{id}/claims`

**Validation:**
- `{id}` must be positive; zero and negative IDs return 400
- `{id}` must identify an existing IdP; unknown positive IDs return 404 before persistence
- At least one claim rule required
- Each claim must have:
  - Non-empty `claim_path` (max 128 chars)
  - Non-empty `value` (max 256 chars)
- Claim paths must be unique within the IdP scope
- Robot claim batches must differ by at least one claim from existing robots

### DELETE Claims Route

#### DELETE `/api/v2.0/federated-idps/{id}/claims`

**Behavior:**
- The `{id}` path parameter is authoritative for every rule in the request
- Classic deletion: delete what exists, silently ignore what doesn't exist
- No 404 errors for non-existing claims
- Returns error only on actual database errors

**Required Fields for Each Claim:**
- `claim_path` - Required
- `robot_id` - Optional (0 = IdP-level, >0 = robot-specific)
- `value` - Optional (for more specific matching)

---

## Testing Checklist

### CREATE Tests
- [x] Valid online mode creation
- [x] Valid offline mode creation
- [x] Name validation (all edge cases)
- [x] URL validation (HTTPS requirement)
- [x] Same issuer with distinct audiences
- [x] `(issuer, audience)` uniqueness (global)
- [x] Name uniqueness (global)
- [x] JWKS keys structure validation
- [x] Online mode fetches discovery
- [x] Derived fields populated correctly

### UPDATE Tests
- [x] Update description (both modes)
- [x] Update jwks_keys (offline only)
- [x] Reject issuer change
- [x] Reject project_id change
- [x] Reject mode switch
- [x] Reject name change

### DELETE Tests
- [x] Delete with no robots
- [x] Reject delete with robots
- [x] Claims cascade deleted

### LIST Tests
- [x] System level query
- [x] Project level query
- [x] Name filter
- [x] Pagination
- [x] Invalid level error

### GET Tests
- [x] Valid ID
- [x] Invalid ID (404)
- [x] Permission check

### Claims API Tests
- [x] Valid claim rule validation
- [x] Claim path length validation (max 128)
- [x] Claim value length validation (max 256)
- [x] Empty claim path rejected
- [x] Empty value rejected
- [x] Nil claim rule rejected
- [x] Batch validation (multiple claims)
- [x] Various claim path patterns (URN, URL, JSONPath, etc.)
- [x] Various claim value patterns (email, JSON, unicode, etc.)
- [x] Edge cases (boundary lengths, special characters)
- [x] Error index reported correctly in batch validation
