-- Migration 0002: add username column to registry_credentials.
-- The initial schema stored only the Kubernetes secret name. M3 adds the
-- username for display in the UI (the plaintext password is never stored here).

ALTER TABLE registry_credentials ADD COLUMN username TEXT NOT NULL DEFAULT '';
