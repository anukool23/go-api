package main

import (
	"github/com/anukool23/olx-api/internal/config"
	"github/com/anukool23/olx-api/internal/db"
	"github/com/anukool23/olx-api/internal/handlers"
	"log"
	"net/http"
	"time"
)

func main() {
	cfg := config.MustLoad()
	_, err := db.Connect(cfg.DbUri)
	if err != nil {
		log.Fatalf("main:db:connect %v",err)
	}
	log.Printf("Database connected successfully...")
	log.Printf("Starting olx server...")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healtz", handlers.Health)

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
