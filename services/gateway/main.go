package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type WorkflowStep struct {
	Key          string                 `json:"key"`
	Type         string                 `json:"type"`
	Position     int                    `json:"position"`
	Configuration map[string]interface{} `json:"configuration"`
}

type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Version     int            `json:"version"`
	Steps       []WorkflowStep `json:"steps"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type Connection struct {
	ID string `json:"id"`
	Provider string `json:"provider"`
	Label string `json:"label"`
	Status string `json:"status"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Run struct {
	ID         string    `json:"id"`
	WorkflowID string    `json:"workflowId"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"createdAt"`
}

type store struct {
	sync.RWMutex
	workflows map[string]Workflow
	runs      map[string][]Run
}

var db = store{workflows: map[string]Workflow{}, runs: map[string][]Run{}}
var durable *postgresStore
var connections = map[string]Connection{}
var encryptedCredentials = map[string][]byte{}
var queue *redisQueue
const defaultWorkspaceID = "00000000-0000-0000-0000-000000000001"

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "flowmation-gateway", "status": "ok"})
}

func validateWithEngine(workflow Workflow) bool {
	engineURL := os.Getenv("FLOWMATION_ENGINE_URL")
	if engineURL == "" { engineURL = "http://localhost:8081" }
	payload, err := json.Marshal(workflow)
	if err != nil { return false }
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Post(engineURL+"/validate", "application/json", bytes.NewReader(payload))
	if err != nil { return false }
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 300
}

func workflows(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/workflows" {
		workflowByID(w, r)
		return
	}
	if r.Method == http.MethodGet {
		if durable != nil { if items, err := durable.listWorkflows(r.Context(), defaultWorkspaceID); err == nil { writeJSON(w, http.StatusOK, items); return } }
		db.RLock()
		items := make([]Workflow, 0, len(db.workflows))
		for _, item := range db.workflows { items = append(items, item) }
		db.RUnlock()
		writeJSON(w, http.StatusOK, items)
		return
	}
	if r.Method == http.MethodPost {
		var input struct { Name string `json:"name"`; Description string `json:"description"`; Steps []WorkflowStep `json:"steps"` }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		now := time.Now().UTC()
		item := Workflow{ID: uuid.NewString(), Name: input.Name, Description: input.Description, Status: "draft", Version: 1, Steps: input.Steps, CreatedAt: now, UpdatedAt: now}
		if durable != nil { if err := durable.saveWorkflow(r.Context(), defaultWorkspaceID, item); err == nil { writeJSON(w, http.StatusCreated, item); return } }
		db.Lock(); db.workflows[item.ID] = item; db.Unlock()
		writeJSON(w, http.StatusCreated, item)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func retryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[3] != "retry" { writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"}); return }
	id := parts[2]
	var previous Run
	var exists bool
	if durable != nil { var err error; previous, exists, err = durable.findRun(r.Context(), id); if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.RLock(); for _, runs := range db.runs { for _, item := range runs { if item.ID == id { previous, exists = item, true } } }; db.RUnlock() }
	if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"}); return }
	var workflow Workflow
	if durable != nil { var err error; workflow, exists, err = durable.getWorkflow(r.Context(), previous.WorkflowID); if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.RLock(); workflow, exists = db.workflows[previous.WorkflowID]; db.RUnlock() }
	if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }
	run := Run{ID: uuid.NewString(), WorkflowID: workflow.ID, Status: "queued", CreatedAt: time.Now().UTC()}
	if durable != nil { if err := durable.saveRun(r.Context(), run); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.Lock(); db.runs[workflow.ID] = append([]Run{run}, db.runs[workflow.ID]...); db.Unlock() }
	if queue != nil { if err := queue.enqueue(r.Context(), run, workflow); err != nil { writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not queue retry"}); return } }
	writeJSON(w, http.StatusAccepted, run)
}

func encryptCredentials(value []byte) ([]byte, error) {
	key := []byte(os.Getenv("FLOWMATION_ENCRYPTION_KEY"))
	if len(key) != 32 { return nil, fmt.Errorf("FLOWMATION_ENCRYPTION_KEY must be 32 bytes") }
	block, err := aes.NewCipher(key); if err != nil { return nil, err }
	gcm, err := cipher.NewGCM(block); if err != nil { return nil, err }
	nonce := make([]byte, gcm.NonceSize()); if _, err = rand.Read(nonce); err != nil { return nil, err }
	return gcm.Seal(nonce, nonce, value, nil), nil
}

func connectionCredentials(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[3] != "credentials" { writeJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"}); return }
	if r.Method != http.MethodPost { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
	var credentials map[string]string
	if json.NewDecoder(r.Body).Decode(&credentials) != nil || len(credentials) == 0 { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credentials are required"}); return }
	payload, err := json.Marshal(credentials); if err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid credentials"}); return }
	ciphertext, err := encryptCredentials(payload); if err != nil { writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()}); return }
	provider := parts[2]; encryptedCredentials[provider] = ciphertext
	item, exists := connections[provider]; if !exists { item = Connection{ID: uuid.NewString(), Provider: provider, Label: provider} }; item.Status = "connected"; item.UpdatedAt = time.Now().UTC(); connections[provider] = item
	writeJSON(w, http.StatusOK, map[string]interface{}{"provider": provider, "status": "connected", "stored": true})
}

func connectionAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet { items := []Connection{}; for _, item := range connections { items = append(items, item) }; writeJSON(w, http.StatusOK, items); return }
	if r.Method == http.MethodPost { var input struct { Provider string `json:"provider"`; Label string `json:"label"` }; if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Provider) == "" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"}); return }; item := Connection{ID: uuid.NewString(), Provider: input.Provider, Label: input.Label, Status: "connected", UpdatedAt: time.Now().UTC()}; connections[item.Provider] = item; writeJSON(w, http.StatusCreated, item); return }
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func allRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
	if durable != nil { if runs, err := durable.listAllRuns(r.Context()); err == nil { writeJSON(w, http.StatusOK, runs); return } }
	db.RLock(); runs := []Run{}; for _, items := range db.runs { runs = append(runs, items...) }; db.RUnlock()
	writeJSON(w, http.StatusOK, runs)
}

func runDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet { writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"}); return }
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 { writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"}); return }
	id := parts[2]
	if durable != nil {
		steps, err := durable.listStepRuns(r.Context(), id)
		if err == nil { writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "steps": steps}); return }
	}
	db.RLock(); defer db.RUnlock()
	for workflowID, runs := range db.runs { for _, run := range runs { if run.ID == id { writeJSON(w, http.StatusOK, map[string]interface{}{"id": id, "workflowId": workflowID, "status": run.Status, "createdAt": run.CreatedAt, "steps": []interface{}{}}); return } } }
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
}

func workflowByID(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }
	id := parts[2]
	if len(parts) == 4 && parts[3] == "run" && r.Method == http.MethodPost {
		var item Workflow
		var exists bool
		if durable != nil { var err error; item, exists, err = durable.getWorkflow(r.Context(), id); if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.RLock(); item, exists = db.workflows[id]; db.RUnlock() }
		if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }
		now := time.Now().UTC()
		status := "queued"
		if !validateWithEngine(item) { status = "failed" }
		run := Run{ID: uuid.NewString(), WorkflowID: id, Status: status, CreatedAt: now}
		if durable != nil { if err := durable.saveRun(r.Context(), run); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.Lock(); db.runs[id] = append([]Run{run}, db.runs[id]...); db.Unlock() }
		if queue != nil && status == "queued" { if err := queue.enqueue(r.Context(), run, item); err != nil { log.Printf("queue enqueue failed: %v", err) } }
		if status == "failed" { writeJSON(w, http.StatusBadGateway, map[string]interface{}{"error": "engine validation failed", "run": run}); return }
		writeJSON(w, http.StatusAccepted, run)
		return
	}
	if len(parts) == 4 && parts[3] == "runs" && r.Method == http.MethodGet {
		if durable != nil { if runs, err := durable.listRuns(r.Context(), id); err == nil { writeJSON(w, http.StatusOK, runs); return } }
		db.RLock(); runs := db.runs[id]; db.RUnlock()
		if runs == nil { runs = []Run{} }
		writeJSON(w, http.StatusOK, runs)
		return
	}
	if r.Method == http.MethodGet {
		if durable != nil { if item, exists, err := durable.getWorkflow(r.Context(), id); err == nil && exists { writeJSON(w, http.StatusOK, item); return } }
		db.RLock(); item, exists := db.workflows[id]; db.RUnlock()
		if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }
		writeJSON(w, http.StatusOK, item)
		return
	}
	if r.Method == http.MethodPut {
		var input struct { Name string `json:"name"`; Description string `json:"description"`; Status string `json:"status"`; Steps []WorkflowStep `json:"steps"` }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"}); return }
		var item Workflow
		var exists bool
		if durable != nil { var err error; item, exists, err = durable.getWorkflow(r.Context(), id); if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.RLock(); item, exists = db.workflows[id]; db.RUnlock() }
		if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }
		item.Name, item.Description, item.Steps, item.UpdatedAt = input.Name, input.Description, input.Steps, time.Now().UTC(); if input.Status != "" { item.Status = input.Status }; item.Version++
		if durable != nil { if err := durable.saveWorkflow(r.Context(), defaultWorkspaceID, item); err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return } } else { db.Lock(); db.workflows[id] = item; db.Unlock() }
		writeJSON(w, http.StatusOK, item); return
	}
	if r.Method == http.MethodDelete {
		if durable != nil { exists, err := durable.deleteWorkflow(r.Context(), id); if err != nil { writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()}); return }; if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }; w.WriteHeader(http.StatusNoContent); return }
		db.Lock(); _, exists := db.workflows[id]; delete(db.workflows, id); delete(db.runs, id); db.Unlock()
		if !exists { writeJSON(w, http.StatusNotFound, map[string]string{"error": "workflow not found"}); return }
		w.WriteHeader(http.StatusNoContent); return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func main() {
	durable, _ = openPostgres(context.Background())
	queue = openQueue()
	now := time.Now().UTC()
	connections["hubspot"] = Connection{ID: "conn_hubspot", Provider: "hubspot", Label: "HubSpot", Status: "connected", UpdatedAt: now}
	connections["slack"] = Connection{ID: "conn_slack", Provider: "slack", Label: "Slack", Status: "connected", UpdatedAt: now}
	db.workflows["wf_demo"] = Workflow{ID: "wf_demo", Name: "New lead enrichment", Description: "Enrich new leads and notify sales", Status: "active", Version: 1, Steps: []WorkflowStep{{Key: "webhook", Type: "webhook", Position: 1}, {Key: "enrich", Type: "action", Position: 2}, {Key: "qualified", Type: "condition", Position: 3}}, CreatedAt: now, UpdatedAt: now}
	http.HandleFunc("/health", health)
	http.Handle("/api/workflows", withHeaders(http.HandlerFunc(workflows)))
	http.Handle("/api/runs", withHeaders(http.HandlerFunc(allRuns)))
	http.Handle("/api/connections", withHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { if strings.HasSuffix(r.URL.Path, "/credentials") { connectionCredentials(w, r); return }; connectionAPI(w, r) })))
	http.Handle("/api/runs/", withHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { if strings.HasSuffix(r.URL.Path, "/retry") { retryRun(w, r); return }; runDetail(w, r) })))
	http.Handle("/api/workflows/", withHeaders(http.HandlerFunc(workflows)))
	log.Println("FlowMation gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
