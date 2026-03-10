# ADR-0003: AWS RDS IAM Authentication

**Status**: Proposed
**Date**: 2026-01-28
**Decision Makers**: Harbor Core Team
**Technical Area**: Database, Security, Cloud Integration
**Depends On**: ADR-0002 (pgxpool migration)

## Context

Organizations running Harbor on AWS (EC2, EKS, ECS) often use RDS or Aurora PostgreSQL as their database backend. Traditional password-based authentication requires:

1. Storing database credentials in configuration files or secrets
2. Manual credential rotation
3. Risk of credential leakage

AWS RDS IAM Authentication provides a more secure alternative by using temporary tokens instead of static passwords. Tokens are:
- Valid for 15 minutes
- Generated using AWS IAM credentials
- Automatically rotated via IAM roles (IRSA, Instance Profiles, Task Roles)

### Requirements

1. **Zero static credentials**: No database password in Harbor configuration
2. **Automatic token refresh**: Connections must remain valid without manual intervention
3. **Multi-environment support**: EC2, EKS (IRSA), ECS, Lambda
4. **Backward compatibility**: Password-based auth must continue working
5. **Migration path**: Existing deployments can switch without downtime

## Decision Drivers

- Security compliance requirements (no long-lived credentials)
- Native AWS integration for managed Kubernetes (EKS)
- Operational simplicity (no credential rotation workflows)
- Leverage pgxpool's `BeforeConnect` hook from ADR-0002

## Options Considered

### Option 1: BeforeConnect Hook with Connection Lifetime (Selected)

Use pgxpool's `BeforeConnect` callback to generate a fresh IAM token for each new connection. Set `MaxConnLifetime` to 14 minutes to ensure connections are recycled before tokens expire.

**Pros**:
- Native pgxpool feature (no external dependencies beyond AWS SDK)
- Simple implementation - token generated per connection
- No caching complexity
- Works with pgxpool metrics/tracing from ADR-0002

**Cons**:
- Token generation on every new connection (minimal overhead)
- Connections recycled every 14 minutes

### Option 2: Token Caching with Background Refresh

Cache the IAM token and refresh it in a background goroutine before expiration. Reuse the same token for multiple connections.

**Pros**:
- Fewer token generation calls
- Connections can live longer

**Cons**:
- Additional complexity (cache management, refresh timing)
- Risk of race conditions between refresh and connection
- Token still expires - connections must eventually recycle

### Option 3: External Token Provider

Delegate token generation to an external service or sidecar (e.g., AWS Secrets Manager rotation, custom token service).

**Pros**:
- Centralized token management
- Could support multiple databases

**Cons**:
- Additional infrastructure component
- Network dependency for token retrieval
- Operational complexity

### Option 4: Pod Restart on Token Expiry

Generate token at startup, terminate the pod before token expires (12 minutes), let Kubernetes restart it.

**Pros**:
- Simple implementation
- No hook complexity

**Cons**:
- Disruptive - causes service interruption
- Not suitable for production
- Wastes resources (container restarts)

## Decision

**Selected: Option 1 - BeforeConnect Hook with Connection Lifetime**

This approach leverages pgxpool's native capabilities introduced in ADR-0002 and provides a clean, maintainable solution.

### Rationale

1. **Simplicity**: No caching, no background goroutines, no external dependencies
2. **Reliability**: Fresh token per connection eliminates stale token issues
3. **pgxpool integration**: Uses `BeforeConnect` hook already available after ADR-0002
4. **14-minute lifetime**: Safe margin before 15-minute token expiry
5. **AWS SDK credential chain**: Automatic credential discovery (IRSA, Instance Profile, etc.)

## Implementation

### Configuration

New configuration options in `database` section:

```yaml
database:
  use_iam_auth: false      # Enable AWS RDS IAM authentication
  aws_region: ""           # AWS region (required if use_iam_auth is true)
```

When `use_iam_auth` is enabled:
- `password` field is ignored
- `ssl_mode` is forced to `require` (IAM auth requires SSL)
- `conn_max_lifetime` is capped at 14 minutes

### Files to Modify

| File | Change |
|------|--------|
| `src/go.mod` | Add `aws-sdk-go-v2` dependencies |
| `src/common/models/database.go` | Add `UseIAMAuth`, `AWSRegion` fields |
| `src/common/dao/pgsql.go` | Add BeforeConnect hook for IAM token |
| `src/lib/config/systemconfig.go` | Load new config options |
| `src/common/const.go` | Add config key constants |
| `make/harbor.yml` | Document new options |

### New Dependencies

| Library | Purpose |
|---------|---------|
| `github.com/aws/aws-sdk-go-v2/config` | AWS configuration and credential chain |
| `github.com/aws/aws-sdk-go-v2/feature/rds/auth` | RDS IAM token generation |

### Interface Changes

The `BeforeConnect` hook signature (from pgx v5):

```go
type BeforeConnect func(ctx context.Context, cfg *pgx.ConnConfig) error
```

### AWS Credential Chain

The implementation uses AWS SDK's default credential chain, supporting:

| Priority | Source | Environment |
|----------|--------|-------------|
| 1 | Environment variables | Any |
| 2 | Shared credentials file | Local dev |
| 3 | Web Identity Token | EKS (IRSA) |
| 4 | EC2 Instance Metadata | EC2 |
| 5 | ECS Task Metadata | ECS |

### Database Migration Support

The `NewMigrator()` function must also support IAM authentication. When `use_iam_auth` is enabled, migrations will generate a fresh token for the migration connection.

## Consequences

### Positive

- No static database credentials in configuration
- Automatic credential rotation via IAM
- Native integration with AWS managed Kubernetes (EKS/IRSA)
- Simplified security compliance
- Works across all AWS compute platforms

### Negative

- AWS-specific feature (not portable to other clouds)
- Requires IAM role configuration in AWS
- Connections recycled every 14 minutes (slight overhead)
- SSL required (minor performance impact)

### Mitigations

- Feature is opt-in (`use_iam_auth: false` by default)
- Clear documentation for IAM role setup
- Connection pooling minimizes reconnection overhead

## Verification

1. **Unit tests**: Mock AWS credential provider
2. **Integration test on EKS**: Deploy with IRSA, verify connections work
3. **Integration test on EC2**: Deploy with Instance Profile
4. **Token expiry test**: Verify connections refresh after 14 minutes
5. **Migration test**: Verify schema migrations work with IAM auth

## Security Considerations

- IAM role must have `rds-db:connect` permission scoped to specific database users
- SSL is mandatory - connections without SSL will be rejected by RDS
- Token generation is audited in AWS CloudTrail
- No credentials stored in Harbor configuration or logs

## Related ADRs

- **ADR-0002**: PostgreSQL Connection Pooling with pgxpool (prerequisite)

## References

- [AWS RDS IAM Authentication](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/UsingWithRDS.IAMDBAuth.html)
- [AWS SDK for Go v2 - RDS Auth](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/feature/rds/auth)
- [EKS IAM Roles for Service Accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html)
- [pgx BeforeConnect Hook](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool#Config)

## Changelog

| Date | Change | Author |
|------|--------|--------|
| 2026-01-28 | Initial proposal | Harbor Team |
