-- Migration 0003: add restart_token column to revisions.
-- Populated only for OperationRestart; carries a unique token into the Helm
-- values so the pod template annotation changes and Kubernetes triggers a rolling
-- restart even when all other values are identical.

ALTER TABLE revisions ADD COLUMN restart_token TEXT NOT NULL DEFAULT '';
