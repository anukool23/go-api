package worker

import (
	"context"
	"database/sql"
	"errors"
	"github/com/anukool23/olx-api/internal/storage"
	"log/slog"
	"time"
)
const (
	maxAttempts = 3
	pollInterval = time.Second * 3
)
type Worker struct {
	db      *sql.DB
	storage *storage.Client
	logger  *slog.Logger
}

func New(db *sql.DB, storage *storage.Client, logger *slog.Logger) *Worker {
	return &Worker{
		db:      db,
		storage: storage,
		logger:  logger,
	}
}

func (w *Worker) Run(ctx context.Context) {
	w.logger.Info("Worker Started")
	for {
		isEmpty , err := w.claimAndProcess(ctx)
		if err != nil {
			w.logger.Error("Job failed", "err", err)
		}
		if isEmpty {
			time.Sleep(pollInterval)
		}
	}
}

func (w *Worker) claimAndProcess(ctx context.Context) (bool, error) {
	var (
		id int64
		kind string
		payload []byte
		attempts int64
	)
	w.logger.Info("Checking new job")
	err := w.db.QueryRowContext(ctx, `
	UPDATE jobs
		SET attempts = attempts + 1,
		run_after = NOW() + LEAST(
			(INTERVAL '30 seconds') * POWER(2, attempts),
			INTERVAL '1 hour')
		WHERE id = (
			SELECT id FROM jobs
			WHERE run_after <= NOW()
			AND attempts < $1
			ORDER BY run_after, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1)
		RETURNING id, kind, payload, attempts`, maxAttempts).Scan(&id, &kind, &payload, &attempts)

		if errors.Is(err, sql.ErrNoRows){
			w.logger.Info("Queue is empty" , "retry", pollInterval)
			return true, nil
		}
		if err != nil {
			return false, err
		}
		w.logger.Info("job processing started", "job id", id, "kind", kind, "attempts", attempts)
	time.Sleep(time.Second)
	return false, nil
}
