package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// postgresStore is the durable workflow store used when DATABASE_URL is configured.
type postgresStore struct {
	db *sql.DB
}

func openPostgres(ctx context.Context) (*postgresStore, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return nil, nil
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &postgresStore{db: db}, nil
}

func (s *postgresStore) close() error { return s.db.Close() }

func (s *postgresStore) saveWorkflow(ctx context.Context, workspaceID string, workflow Workflow) error {
	var err error
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO workflows (id, workspace_id, name, description, status, version)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description,
		status = EXCLUDED.status, version = EXCLUDED.version, updated_at = NOW()`,
		workflow.ID, workspaceID, workflow.Name, workflow.Description, workflow.Status, workflow.Version)
	if err != nil { return fmt.Errorf("save workflow: %w", err) }
	_, err = s.db.ExecContext(ctx, `DELETE FROM workflow_steps WHERE workflow_id = $1`, workflow.ID)
	if err != nil { return fmt.Errorf("replace workflow steps: %w", err) }
	for _, step := range workflow.Steps {
		config, encodeErr := json.Marshal(step.Configuration)
		if encodeErr != nil { return fmt.Errorf("encode step configuration: %w", encodeErr) }
		if _, err = s.db.ExecContext(ctx, `INSERT INTO workflow_steps (workflow_id, step_key, step_type, position, configuration) VALUES ($1, $2, $3, $4, $5)`, workflow.ID, step.Key, step.Type, step.Position, config); err != nil {
			return fmt.Errorf("save workflow step: %w", err)
		}
	}
	return nil
}

func (s *postgresStore) listWorkflows(ctx context.Context, workspaceID string) ([]Workflow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, status, version, created_at, updated_at FROM workflows WHERE workspace_id = $1 ORDER BY updated_at DESC`, workspaceID)
	if err != nil { return nil, fmt.Errorf("list workflows: %w", err) }
	defer rows.Close()
	items := []Workflow{}
	for rows.Next() {
		var item Workflow
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt); err != nil { return nil, fmt.Errorf("scan workflow: %w", err) }
		stepRows, stepErr := s.db.QueryContext(ctx, `SELECT step_key, step_type, position, configuration FROM workflow_steps WHERE workflow_id = $1 ORDER BY position`, item.ID)
		if stepErr != nil { return nil, fmt.Errorf("list workflow steps: %w", stepErr) }
		for stepRows.Next() {
			var step WorkflowStep
			var config []byte
			if scanErr := stepRows.Scan(&step.Key, &step.Type, &step.Position, &config); scanErr != nil { stepRows.Close(); return nil, fmt.Errorf("scan workflow step: %w", scanErr) }
			if decodeErr := json.Unmarshal(config, &step.Configuration); decodeErr != nil { stepRows.Close(); return nil, fmt.Errorf("decode workflow step: %w", decodeErr) }
			item.Steps = append(item.Steps, step)
		}
		stepRows.Close()
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStore) getWorkflow(ctx context.Context, id string) (Workflow, bool, error) {
	var item Workflow
	err := s.db.QueryRowContext(ctx, `SELECT id, name, description, status, version, created_at, updated_at FROM workflows WHERE id = $1`, id).Scan(&item.ID, &item.Name, &item.Description, &item.Status, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows { return Workflow{}, false, nil }
	if err != nil { return Workflow{}, false, fmt.Errorf("get workflow: %w", err) }
	rows, err := s.db.QueryContext(ctx, `SELECT step_key, step_type, position, configuration FROM workflow_steps WHERE workflow_id = $1 ORDER BY position`, id)
	if err != nil { return Workflow{}, false, fmt.Errorf("get workflow steps: %w", err) }
	defer rows.Close()
	for rows.Next() {
		var step WorkflowStep
		var config []byte
		if err := rows.Scan(&step.Key, &step.Type, &step.Position, &config); err != nil { return Workflow{}, false, fmt.Errorf("scan workflow step: %w", err) }
		if err := json.Unmarshal(config, &step.Configuration); err != nil { return Workflow{}, false, fmt.Errorf("decode workflow step: %w", err) }
		item.Steps = append(item.Steps, step)
	}
	return item, true, rows.Err()
}

func (s *postgresStore) deleteWorkflow(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workflows WHERE id = $1`, id)
	if err != nil { return false, fmt.Errorf("delete workflow: %w", err) }
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *postgresStore) saveRun(ctx context.Context, run Run) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_runs (id, workflow_id, status) VALUES ($1, $2, $3)`, run.ID, run.WorkflowID, run.Status)
	if err != nil { return fmt.Errorf("save run: %w", err) }
	return nil
}

func (s *postgresStore) listRuns(ctx context.Context, workflowID string) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workflow_id, status, created_at FROM workflow_runs WHERE workflow_id = $1 ORDER BY created_at DESC`, workflowID)
	if err != nil { return nil, fmt.Errorf("list runs: %w", err) }
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var run Run
		if err := rows.Scan(&run.ID, &run.WorkflowID, &run.Status, &run.CreatedAt); err != nil { return nil, fmt.Errorf("scan run: %w", err) }
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *postgresStore) listAllRuns(ctx context.Context) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workflow_id, status, created_at FROM workflow_runs ORDER BY created_at DESC LIMIT 100`)
	if err != nil { return nil, fmt.Errorf("list all runs: %w", err) }
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		var run Run
		if err := rows.Scan(&run.ID, &run.WorkflowID, &run.Status, &run.CreatedAt); err != nil { return nil, fmt.Errorf("scan all run: %w", err) }
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *postgresStore) listStepRuns(ctx context.Context, runID string) ([]map[string]interface{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT ws.step_key, ws.step_type, wsr.status, wsr.started_at, wsr.finished_at, wsr.error_message FROM workflow_step_runs wsr JOIN workflow_steps ws ON ws.id = wsr.step_id WHERE wsr.run_id = $1 ORDER BY ws.position`, runID)
	if err != nil { return nil, fmt.Errorf("list step runs: %w", err) }
	defer rows.Close()
	items := []map[string]interface{}{}
	for rows.Next() {
		var key, stepType, status string
		var started, finished interface{}
		var message *string
		if err := rows.Scan(&key, &stepType, &status, &started, &finished, &message); err != nil { return nil, fmt.Errorf("scan step run: %w", err) }
		items = append(items, map[string]interface{}{"key": key, "type": stepType, "status": status, "startedAt": started, "finishedAt": finished, "error": message})
	}
	return items, rows.Err()
}

func (s *postgresStore) findRun(ctx context.Context, id string) (Run, bool, error) {
	var run Run
	err := s.db.QueryRowContext(ctx, `SELECT id, workflow_id, status, created_at FROM workflow_runs WHERE id = $1`, id).Scan(&run.ID, &run.WorkflowID, &run.Status, &run.CreatedAt)
	if err == sql.ErrNoRows { return Run{}, false, nil }
	if err != nil { return Run{}, false, fmt.Errorf("find run: %w", err) }
	return run, true, nil
}
