/*
  Migrate legacy BuildKit attestation references to accessories.

  BuildKit (Docker 24+) embeds attestation manifests (SLSA provenance, SPDX
  SBOM) as entries within OCI Image Indexes. Harbor previously stored these as
  regular artifact_reference rows with platform unknown/unknown, causing
  confusing UI rows.

  This migration:
  1. Finds artifact_reference rows annotated as attestation-manifest
  2. Resolves the subject platform artifact via the annotation digest
  3. Inserts corresponding artifact_accessory records (type attestation.buildkit)
  4. Removes the migrated artifact_reference rows

  Performance: the WHERE clause filters on the jsonb annotations column, which
  limits the working set to only attestation rows. The JOINs use indexed
  columns (id, repository_name+digest). Safe for large registries.

  Idempotent: ON CONFLICT DO NOTHING prevents duplicates if run again.
*/

/* Step 1: Create accessory records from attestation references */
INSERT INTO artifact_accessory (
    artifact_id,
    subject_artifact_id,
    subject_artifact_repo,
    subject_artifact_digest,
    type,
    size,
    digest
)
SELECT
    ar.child_id,
    a_subject.id,
    a_parent.repository_name,
    ar.annotations->>'vnd.docker.reference.digest',
    'attestation.buildkit',
    a_child.size,
    a_child.digest
FROM artifact_reference ar
JOIN artifact a_parent
  ON a_parent.id = ar.parent_id
JOIN artifact a_child
  ON a_child.id = ar.child_id
JOIN artifact a_subject
  ON a_subject.repository_name = a_parent.repository_name
 AND a_subject.digest = ar.annotations->>'vnd.docker.reference.digest'
WHERE ar.annotations->>'vnd.docker.reference.type' = 'attestation-manifest'
  AND ar.annotations->>'vnd.docker.reference.digest' IS NOT NULL
ON CONFLICT DO NOTHING;

/* Step 2: Remove migrated reference rows */
DELETE FROM artifact_reference
WHERE id IN (
    SELECT ar.id
    FROM artifact_reference ar
    JOIN artifact a_parent
      ON a_parent.id = ar.parent_id
    JOIN artifact a_subject
      ON a_subject.repository_name = a_parent.repository_name
     AND a_subject.digest = ar.annotations->>'vnd.docker.reference.digest'
    WHERE ar.annotations->>'vnd.docker.reference.type' = 'attestation-manifest'
      AND ar.annotations->>'vnd.docker.reference.digest' IS NOT NULL
);
