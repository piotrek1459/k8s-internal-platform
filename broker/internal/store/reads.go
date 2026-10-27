package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"k8s-internal-platform/broker/internal/platform"
)

// scanApplication reads one application row. The scanner is a *sql.Row or *sql.Rows.
func scanApplication(sc interface{ Scan(...any) error }) (platform.Application, error) {
	var (
		app                         platform.Application
		desired, lastSucc, activeOp sql.NullString
		deleted                     int
		createdAt, updatedAt        string
	)
	if err := sc.Scan(&app.ID, &app.Name, &app.Version, &desired, &lastSucc, &activeOp,
		&deleted, &createdAt, &updatedAt); err != nil {
		return platform.Application{}, err
	}
	app.DesiredRevisionID = desired.String
	app.LastSuccessfulRevision = lastSucc.String
	app.ActiveOperationID = activeOp.String
	app.Deleted = deleted != 0
	app.CreatedAt = parseTime(createdAt)
	app.UpdatedAt = parseTime(updatedAt)
	return app, nil
}

const applicationColumns = `id, name, version, desired_revision_id, last_successful_revision,
	active_operation_id, deleted, created_at, updated_at`

func (s *Store) GetApplication(ctx context.Context, id string) (platform.Application, error) {
	row := s.db.sql.QueryRowContext(ctx,
		`SELECT `+applicationColumns+` FROM applications WHERE id = ?`, id)
	app, err := scanApplication(row)
	if err != nil {
		return platform.Application{}, notFound(err, "application", id)
	}
	return app, nil
}

func (s *Store) ListApplications(ctx context.Context, limit, offset int) ([]platform.Application, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT `+applicationColumns+` FROM applications WHERE deleted = 0
		 ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.Application
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, app)
	}
	return out, rows.Err()
}

func scanRevision(sc interface{ Scan(...any) error }) (platform.Revision, error) {
	var (
		rev          platform.Revision
		specJSON     string
		frozen       int
		imageDigest  sql.NullString
		chartVersion sql.NullString
		chartDigest  sql.NullString
		secretVers   sql.NullString
		valuesHash   sql.NullString
		frozenAt     sql.NullString
		result       string
		createdAt    string
	)
	if err := sc.Scan(&rev.ID, &rev.AppID, &rev.Number, &specJSON, &frozen,
		&imageDigest, &chartVersion, &chartDigest, &secretVers, &valuesHash, &frozenAt,
		&result, &rev.Actor, &createdAt); err != nil {
		return platform.Revision{}, err
	}
	if err := json.Unmarshal([]byte(specJSON), &rev.Spec); err != nil {
		return platform.Revision{}, err
	}
	rev.Result = platform.RevisionResult(result)
	rev.Created = parseTime(createdAt)
	if frozen != 0 {
		fs := platform.FrozenSpec{
			ImageDigest:         imageDigest.String,
			ChartVersion:        chartVersion.String,
			ChartArtifactDigest: chartDigest.String,
			ValuesHash:          valuesHash.String,
			FrozenAt:            parseTime(frozenAt.String),
		}
		if secretVers.Valid && secretVers.String != "" {
			if err := json.Unmarshal([]byte(secretVers.String), &fs.SecretVersions); err != nil {
				return platform.Revision{}, err
			}
		}
		rev.Frozen = &fs
	}
	return rev, nil
}

const revisionColumns = `id, app_id, number, spec_json, frozen, image_digest, chart_version,
	chart_artifact_digest, secret_versions_json, values_hash, frozen_at, result, actor, created_at`

func (s *Store) GetRevision(ctx context.Context, id string) (platform.Revision, error) {
	row := s.db.sql.QueryRowContext(ctx,
		`SELECT `+revisionColumns+` FROM revisions WHERE id = ?`, id)
	rev, err := scanRevision(row)
	if err != nil {
		return platform.Revision{}, notFound(err, "revision", id)
	}
	return rev, nil
}

func (s *Store) ListRevisions(ctx context.Context, appID string) ([]platform.Revision, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT `+revisionColumns+` FROM revisions WHERE app_id = ? ORDER BY number DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.Revision
	for rows.Next() {
		rev, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func scanOperation(sc interface{ Scan(...any) error }) (platform.Operation, error) {
	var (
		op                    platform.Operation
		source, idemKey       sql.NullString
		reqHash               sql.NullString
		createdAt, updatedAt  string
		deadlineAt, claimedAt sql.NullString
		state, typ            string
	)
	if err := sc.Scan(&op.ID, &op.AppID, &typ, &source, &op.TargetRevision, &state,
		&op.Stage, &idemKey, &reqHash, &op.RecoveryOutcome, &createdAt, &updatedAt,
		&deadlineAt, &claimedAt); err != nil {
		return platform.Operation{}, err
	}
	op.Type = platform.OperationType(typ)
	op.State = platform.OperationState(state)
	op.SourceRevision = source.String
	op.IdempotencyKey = idemKey.String
	op.RequestHash = reqHash.String
	op.CreatedAt = parseTime(createdAt)
	op.UpdatedAt = parseTime(updatedAt)
	op.DeadlineAt = parseTimePtr(deadlineAt)
	op.ClaimedAt = parseTimePtr(claimedAt)
	return op, nil
}

const operationColumns = `id, app_id, type, source_revision, target_revision, state, stage,
	idempotency_key, request_hash, recovery_outcome, created_at, updated_at, deadline_at, claimed_at`

func (s *Store) GetOperation(ctx context.Context, id string) (platform.Operation, error) {
	row := s.db.sql.QueryRowContext(ctx,
		`SELECT `+operationColumns+` FROM operations WHERE id = ?`, id)
	op, err := scanOperation(row)
	if err != nil {
		return platform.Operation{}, notFound(err, "operation", id)
	}
	return op, nil
}

func (s *Store) ListOperationEvents(ctx context.Context, operationID string) ([]platform.OperationEvent, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT id, operation_id, at, stage, message, request_id FROM operation_events
		 WHERE operation_id = ? ORDER BY id`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.OperationEvent
	for rows.Next() {
		var ev platform.OperationEvent
		var at string
		if err := rows.Scan(&ev.ID, &ev.OperationID, &at, &ev.Stage, &ev.Message, &ev.RequestID); err != nil {
			return nil, err
		}
		ev.At = parseTime(at)
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) ListNonterminalOperations(ctx context.Context) ([]platform.Operation, error) {
	rows, err := s.db.sql.QueryContext(ctx,
		`SELECT `+operationColumns+` FROM operations
		 WHERE state NOT IN (?, ?) ORDER BY created_at`,
		string(platform.OperationSucceeded), string(platform.OperationFailed))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []platform.Operation
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}
