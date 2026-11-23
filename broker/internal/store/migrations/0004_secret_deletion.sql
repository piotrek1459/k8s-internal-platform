-- Reserve deletion before crossing the SQLite/Kubernetes boundary. A failed
-- cleanup remains retryable and cannot acquire new application references.
ALTER TABLE secrets ADD COLUMN deleting INTEGER NOT NULL DEFAULT 0 CHECK (deleting IN (0, 1));
