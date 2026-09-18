package main

import (
	"github/com/anukool23/olx-api/internal/config"
	"github/com/anukool23/olx-api/internal/db"
	"github/com/anukool23/olx-api/internal/handlers"
	"github/com/anukool23/olx-api/internal/middleware"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	cfg := config.MustLoad()
	db, err := db.Connect(cfg.DbUri)
	if err != nil {
		log.Fatalf("main:db:connect %v", err)
	}
	log.Printf("Database connected successfully...")
	log.Printf("Starting olx server...")

	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level: slog.LevelDebug,
	})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	lh := handlers.NewListingHandler(db, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healtz", handlers.Health)
	mux.HandleFunc("GET /listings", lh.List)
	mux.HandleFunc("DELETE /listings/{id}", lh.Delete)
	mux.HandleFunc("POST /listings", lh.Create)

	handler := middleware.RequestId(mux)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("Server is started at %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start %v", err)
	}

}
