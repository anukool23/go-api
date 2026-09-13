package main

import (
	"github/com/anukool23/olx-api/internal/config"
	"log"
	"net/http"
	"time"
)

func main() {
	cfg := config.MustLoad()
	log.Printf("Starting olx server...")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healtz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("Server is started at %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start %v", err)
	}

}
