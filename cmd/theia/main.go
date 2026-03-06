package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	listenAddr := os.Getenv("THEIA_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	http.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"name":   "mikrotik-theia",
		})
	})

	fmt.Printf("MikroTik Theia starting on %s\n", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		log.Fatal(err)
	}
}
