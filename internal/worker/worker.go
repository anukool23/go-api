package worker

import (
	"database/sql"
	"github/com/anukool23/olx-api/internal/storage"
	"log/slog"
)

type Worker struct {
	db      *sql.DB
	storage *storage.Client
	logger  *slog.Logger
}

func New(db *sql.DB, storage *storage.Client, logger *slog.Logger) *Worker{
	return &Worker{
		db: db,
		storage: storage,
		logger: logger,
	}
}

func (w *Worker)Run() {
	w.logger.Info("Worker Started")
}