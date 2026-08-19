Feature: OTLP audit forwarding

  @audit @audit-otlp
  Scenario: Request source reaches OTLP for API mutations and manifest pulls
    Given a running Harbor
    And an in-process OTLP audit collector
    When the admin enables OTLP audit forwarding from client IP "198.51.100.24" and user agent "harbor-e2e-api/1.0"
    Then the collector receives audit event "harbor.audit.update_configuration" with client IP "198.51.100.24" and user agent "harbor-e2e-api/1.0"
    Given a fresh project "audit-source"
    And an image pushed to "audit-source/app:v1"
    When a client pulls manifest "audit-source/app:v1" from client IP "203.0.113.42" and user agent "containerd-e2e/2.0"
    Then the collector receives a pull audit for "audit-source/app:v1" with client IP "203.0.113.42" and user agent "containerd-e2e/2.0"
