-- Migration 0001: initial platform schema.
--
-- Maps the entities in docs/api-and-data.md. Foreign keys use ON DELETE RESTRICT
-- so retention or deletion cannot orphan a revision, operation, or secret version
-- that a nonterminal (NeedsAttention) operation still depends on. SQLite enforces
-- foreign keys only when PRAGMA foreign_keys=ON, which the store sets per
-- connection.

CREATE TABLE administrators (
    id            TEXT PRIMARY KEY,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,           -- hashed session identifier
    admin_id   TEXT NOT NULL REFERENCES administrators(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE TABLE applications (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL UNIQUE,
    version                  INTEGER NOT NULL DEFAULT 1,  -- optimistic lock / ETag
    desired_revision_id      TEXT,
    last_successful_revision TEXT,
    active_operation_id      TEXT,                        -- 409 lock when non-null
    deleted                  INTEGER NOT NULL DEFAULT 0,
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);

CREATE TABLE revisions (
    id                    TEXT PRIMARY KEY,
    app_id                TEXT NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    number                INTEGER NOT NULL,
    -- Requested (mutable) specification, stored as JSON.
    spec_json             TEXT NOT NULL,
    -- Frozen (immutable) execution specification, set before any cluster mutation.
    frozen                INTEGER NOT NULL DEFAULT 0,
    image_digest          TEXT,
    chart_version         TEXT,
    chart_artifact_digest TEXT,
    secret_versions_json  TEXT,
    values_hash           TEXT,
    frozen_at             TEXT,
    result                TEXT NOT NULL DEFAULT 'pending',
    actor                 TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    UNIQUE (app_id, number)
);

CREATE TABLE operations (
    id               TEXT PRIMARY KEY,
    app_id           TEXT NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    type             TEXT NOT NULL,
    source_revision  TEXT,
    target_revision  TEXT NOT NULL REFERENCES revisions(id) ON DELETE RESTRICT,
    state            TEXT NOT NULL,
    stage            TEXT NOT NULL DEFAULT '',
    idempotency_key  TEXT,
    request_hash     TEXT,
    recovery_outcome TEXT NOT NULL DEFAULT '',
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL,
    deadline_at      TEXT,
    claimed_at       TEXT,
    -- An idempotency key is unique per application, so a retried submission maps
    -- back to its original operation instead of creating a second one.
    UNIQUE (app_id, idempotency_key)
);

CREATE INDEX operations_state_idx ON operations(state);
CREATE INDEX operations_app_idx ON operations(app_id);

CREATE TABLE operation_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    operation_id TEXT NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    at           TEXT NOT NULL,
    stage        TEXT NOT NULL,
    message      TEXT NOT NULL,
    request_id   TEXT NOT NULL DEFAULT ''
);

CREATE INDEX operation_events_op_idx ON operation_events(operation_id, id);

CREATE TABLE audit_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    actor        TEXT NOT NULL,
    action       TEXT NOT NULL,
    target       TEXT NOT NULL,
    at           TEXT NOT NULL,
    outcome      TEXT NOT NULL,
    request_id   TEXT NOT NULL DEFAULT '',
    operation_id TEXT NOT NULL DEFAULT '',
    diff         TEXT NOT NULL DEFAULT ''
);

CREATE TABLE registry_credentials (
    id                 TEXT PRIMARY KEY,
    registry           TEXT NOT NULL,
    label              TEXT NOT NULL,
    k8s_secret_name    TEXT NOT NULL,
    verification_status TEXT NOT NULL DEFAULT 'unverified',
    created_at         TEXT NOT NULL
);

CREATE TABLE secrets (
    id         TEXT PRIMARY KEY,
    owner      TEXT NOT NULL,
    name       TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE secret_versions (
    secret_id       TEXT NOT NULL REFERENCES secrets(id) ON DELETE RESTRICT,
    version         INTEGER NOT NULL,
    keys_json       TEXT NOT NULL,     -- key names only; never values
    k8s_secret_name TEXT NOT NULL,
    k8s_secret_uid  TEXT NOT NULL,
    created_at      TEXT NOT NULL,
    PRIMARY KEY (secret_id, version)
);

CREATE TABLE volumes (
    id             TEXT PRIMARY KEY,
    pvc_name       TEXT NOT NULL,
    requested_size TEXT NOT NULL,
    app_id         TEXT,
    retained       INTEGER NOT NULL DEFAULT 1,
    deleted        INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL
);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE observed_status (
    app_id     TEXT PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
    status_json TEXT NOT NULL,
    observed_at TEXT NOT NULL
);
