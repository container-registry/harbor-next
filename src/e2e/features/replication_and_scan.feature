Feature: Replication and vulnerability scanning

  Background:
    Given a running Harbor

  @replication
  Scenario: A Quay.io tag is replicated to Harbor on demand
    Given a Quay.io endpoint is registered
    And a replication policy from Quay.io matching "prometheus/node-exporter:v1.9.0"
    When the admin triggers the replication policy
    Then the execution reports success
    And the replicated artifact in Harbor matches the upstream digest

  @replication
  Scenario: A non-matching replication filter produces zero tasks
    Given a Quay.io endpoint is registered
    And a replication policy from Quay.io matching a pattern with no upstream match
    When the admin triggers the replication policy
    Then the execution reports success with zero tasks

  @replication @sftp
  Scenario: An image can be backed up to SFTP and restored after local deletion
    Given a fresh project "source"
    And an image pushed to "source/app:v1"
    And an SFTP endpoint is registered
    And the SFTP endpoint reports healthy
    And invalid SFTP credentials are rejected
    When the admin replicates "source/app:v1" to SFTP
    Then the execution reports success
    And the SFTP storage contains the replicated image "source/app:v1"
    When the admin deletes "source"
    Given a fresh project "restore"
    When the admin restores "source/app:v1" from SFTP into "restore/app:v1"
    Then the execution reports success
    When the admin pulls "restore/app:v1"
    Then the pulled digest matches the pushed digest

  @replication @sftp
  Scenario: Event-based replication deletes a tag from SFTP
    Given a fresh project "source"
    And an image pushed to "source/app:v1"
    And an SFTP endpoint is registered
    When the admin replicates "source/app:v1" to SFTP
    Then the execution reports success
    And the SFTP storage contains the replicated image "source/app:v1"
    Given an event-based SFTP deletion replication policy for "source/app:v1"
    When the admin deletes tag "v1"
    Then the event-based deletion replication execution reports success
    And the SFTP storage no longer contains the replicated image "source/app:v1"

  @smoke @scan
  Scenario: Trivy returns a scan report with severities within the timeout
    Given a fresh project "proj"
    And an image pushed to "proj/app:v1"
    When the admin triggers a scan on "proj/app:v1"
    Then the scan report completes within 60 seconds
    And the report includes a severity summary and at least one CVE
