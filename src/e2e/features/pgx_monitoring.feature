Feature: pgx database monitoring metrics (patch 0005)
  As a Harbor operator
  I want core and jobservice to expose pgx database metrics
  So that database pool and query behaviour can be monitored safely

  # Patch 0005 adds POSTGRESQL_METRICS_ENABLED and bridges pgx/pgxpool OTel
  # metrics into Harbor's existing Prometheus metric endpoints.

  @pgx-monitoring @observability
  Scenario: Core and JobService expose pgx database metrics without leaking resource names
    Given a running Harbor
    When I exercise PostgreSQL-backed Harbor APIs
    And I run a PostgreSQL-backed jobservice task
    And I scrape database metrics from core and jobservice
    Then both services expose pgx pool metrics
    And both services expose pgx database operation metrics
    And pgx metrics do not expose scenario resource names
