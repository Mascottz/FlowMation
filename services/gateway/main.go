package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(healthResponse{Service: "flowmation-gateway", Status: "ok"})
}

func main() {
	http.HandleFunc("/health", health)
	log.Println("FlowMation gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
