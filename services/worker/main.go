package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type workflowStep struct { Key string `json:"key"` }
type workflowPayload struct { Steps []workflowStep `json:"steps"` }
type runMessage struct { Run struct { ID string `json:"id"`; WorkflowID string `json:"workflowId"` } `json:"run"`; Workflow json.RawMessage `json:"workflow"` }


func persistStepRuns(database *sql.DB, runID, workflowID, status string, payload []byte) {
	var workflow workflowPayload
	if json.Unmarshal(payload, &workflow) != nil { return }
	for _, step := range workflow.Steps {
		var stepID string
		if err := database.QueryRow(`SELECT id FROM workflow_steps WHERE workflow_id = $1 AND step_key = $2`, workflowID, step.Key).Scan(&stepID); err != nil { continue }
		_, _ = database.Exec(`INSERT INTO workflow_step_runs (run_id, step_id, status, started_at, finished_at) VALUES ($1, $2, $3, NOW(), NOW())`, runID, stepID, status)
	}
}

func main() {
	url := os.Getenv("REDIS_URL")
	if url == "" { url = "redis://localhost:6379/0" }
	options, err := redis.ParseURL(url); if err != nil { log.Fatal(err) }
	queue := redis.NewClient(options)
	engineURL := os.Getenv("FLOWMATION_ENGINE_URL"); if engineURL == "" { engineURL = "http://engine:8081" }
	var database *sql.DB
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" { database, err = sql.Open("pgx", databaseURL); if err != nil { log.Fatal(err) }; if err = database.Ping(); err != nil { log.Fatal(err) }; defer database.Close() }
	log.Println("FlowMation worker is listening for workflow runs")
	for {
		result, err := queue.BLPop(context.Background(), 0, "flowmation:runs").Result()
		if err != nil { log.Printf("queue read failed: %v", err); time.Sleep(time.Second); continue }
		var message runMessage
		if err := json.Unmarshal([]byte(result[1]), &message); err != nil { log.Printf("invalid run payload: %v", err); continue }
		if database != nil { if _, updateErr := database.Exec(`UPDATE workflow_runs SET status = 'running', started_at = NOW() WHERE id = $1`, message.Run.ID); updateErr != nil { log.Printf("could not mark run running: %v", updateErr) } }
		request, _ := http.NewRequest(http.MethodPost, engineURL+"/validate", strings.NewReader(string(message.Workflow)))
		request.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Do(request)
		status := "failed"
		if err == nil && response.StatusCode >= 200 && response.StatusCode < 300 { response.Body.Close(); executeRequest, _ := http.NewRequest(http.MethodPost, engineURL+"/execute", strings.NewReader(string(message.Workflow))); executeRequest.Header.Set("Content-Type", "application/json"); executeResponse, executeErr := client.Do(executeRequest); if executeErr == nil && executeResponse.StatusCode >= 200 && executeResponse.StatusCode < 300 { status = "succeeded" }; if executeResponse != nil { executeResponse.Body.Close() } }
		if database != nil { var executionError interface{}; if status == "failed" { executionError = "Rust engine execution failed" }; if result, updateErr := database.Exec(`UPDATE workflow_runs SET status = $1, finished_at = NOW(), error_message = $2 WHERE id = $3`, status, executionError, message.Run.ID); updateErr != nil { log.Printf("could not finalize run: %v", updateErr) } else if affected, rowsErr := result.RowsAffected(); rowsErr == nil && affected == 0 { log.Printf("run %s was not found while finalizing", message.Run.ID) }; persistStepRuns(database, message.Run.ID, message.Run.WorkflowID, status, message.Workflow) }
		_ = queue.Set(context.Background(), "flowmation:run:"+message.Run.ID, status, 24*time.Hour).Err()
		log.Printf("workflow run %s completed with status %s", message.Run.ID, status)
	}
}

