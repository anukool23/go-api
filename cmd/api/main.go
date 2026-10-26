package main

import (
	"context"
	"github/com/anukool23/olx-api/internal/config"
	"github/com/anukool23/olx-api/internal/db"
	"github/com/anukool23/olx-api/internal/handlers"
	"github/com/anukool23/olx-api/internal/middleware"
	"github/com/anukool23/olx-api/internal/storage"
	"github/com/anukool23/olx-api/internal/worker"
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

	storage, err := storage.NewR2(context.TODO(), storage.R2Config{
		AccountId:       cfg.StorageAccountId,
		AccessKeyID:     cfg.StorageAccessKey,
		AccessKeySecret: cfg.StorageAccessSecret,
		Bucket:          cfg.StorageBucket,
	})
	if err != nil {
		log.Fatalf("main:storage:r2 %v", err)
	}
	log.Printf("Storage initalized")

	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	wrker := worker.New(db, storage, logger)
	go wrker.Run(context.TODO())
	log.Print("Worker started")

	log.Printf("Starting olx server...")

	lh := handlers.NewListingHandler(db, logger, storage)
	ah := handlers.NewAuthHandler(db, logger, cfg)
	uh := handlers.NewUploadHandler(logger, storage)

	authMiddleware := middleware.RequireAuth(logger, cfg.JwtKey)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healtz", handlers.Health)
	mux.HandleFunc("GET /listings", lh.List)
	mux.Handle("DELETE /listings/{id}", authMiddleware(http.HandlerFunc(lh.Delete)))
	mux.Handle("POST /listings", authMiddleware(http.HandlerFunc(lh.Create)))
	mux.Handle("GET /mylistings", authMiddleware(http.HandlerFunc(lh.MyListings)))
	mux.HandleFunc("POST /signup", ah.SignUp)
	mux.HandleFunc("POST /signin", ah.SignIn)
	mux.Handle("POST /uploads/presign", authMiddleware(http.HandlerFunc(uh.Presign)))

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
