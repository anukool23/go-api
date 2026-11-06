package main

import (
	"context"
	"errors"
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
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/time/rate"
)

const (
	serverShutDownTimeout = 15 * time.Second
	workerShutDownTimeout = 10 * time.Second
)

func main() {
	cfg := config.MustLoad()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
	go wrker.Run(ctx)
	log.Print("Worker started")

	log.Printf("Starting olx server...")

	lh := handlers.NewListingHandler(db, logger, storage)
	ah := handlers.NewAuthHandler(db, logger, cfg)
	uh := handlers.NewUploadHandler(logger, storage)

	globalRateLimit := middleware.RateLimit(logger, rate.Every(time.Second), 100)
	signInLimiter := middleware.RateLimit(logger, rate.Every(time.Second*12), 5)
	signUpLimiter := middleware.RateLimit(logger, rate.Every(time.Minute), 2)

	authMiddleware := middleware.RequireAuth(logger, cfg.JwtKey)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healtz", handlers.Health)
	mux.HandleFunc("GET /listings", lh.List)
	mux.HandleFunc("GET /listings/{id}", lh.ListingDetail)
	mux.Handle("DELETE /listings/{id}", authMiddleware(http.HandlerFunc(lh.Delete)))
	mux.Handle("POST /listings", authMiddleware(http.HandlerFunc(lh.Create)))
	mux.Handle("GET /mylistings", authMiddleware(http.HandlerFunc(lh.MyListings)))
	mux.Handle("POST /signup", signUpLimiter(http.HandlerFunc(ah.SignUp)))
	mux.Handle("POST /signin", signInLimiter(http.HandlerFunc(ah.SignIn)))
	mux.Handle("POST /uploads/presign", authMiddleware(http.HandlerFunc(uh.Presign)))

	handler := globalRateLimit(middleware.RequestId(mux))
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Printf("Server is started at %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start %v", err)
		}
	}()

	<-ctx.Done()
	stop()

	logger.Info("Shutdown started")
	srvShutDownCtx, cancel := context.WithTimeout(context.Background(), serverShutDownTimeout)
	defer cancel()
	if err := srv.Shutdown(srvShutDownCtx); err != nil {
		logger.Error("Main:Server Graceful Shutdown failed", "err", err)
	}

	select {
	case <-wrker.Done():
	case <-time.After(workerShutDownTimeout):
		logger.Warn("Worker is still busy, Exiting anyway")
	}

	db.Close()
	logger.Info("Shutdown completed....")
}
