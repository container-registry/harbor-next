# ADR-0005: Route Federated IdPs by Issuer and Audience

**Status**: Accepted
**Date**: 2026-07-27
**Decision Makers**: 8gcr Team
**Technical Area**: Federated Authentication

## Context

Multiple tenants can use one OIDC issuer while receiving tokens for different client audiences. Routing solely by issuer prevents these tenants from configuring separate system- or project-level IdPs.

## Decision

Store the audience as one immutable IdP-level `claim_rules` entry, rather than a column on `identity_providers`. IdP names remain globally unique; issuers may repeat; the exact `(issuer, audience)` combination is globally unique.

Authentication reads `iss` and `aud` before resolving keys. It accepts exactly one matching IdP and fails closed for missing audiences, incomplete IdPs, and ambiguous legacy configurations. Both string and array JWT audiences are supported.

IdPs continue to be created in two calls. No audience is backfilled for existing IdPs, so an audience-less IdP remains unable to authenticate until configured.

## Consequences

- No database migration is required; audience routing remains in application-managed claim rules.
- `aud` and `iss` are reserved, immutable IdP-level identity claims.
- The UI requires `aud` and `iss` during creation; the audience is locked after creation.
