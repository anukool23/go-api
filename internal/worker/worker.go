package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github/com/anukool23/olx-api/internal/imaging"
	"github/com/anukool23/olx-api/internal/jobs"
	"github/com/anukool23/olx-api/internal/storage"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

const (
	maxAttempts  = 3
	pollInterval = time.Second * 3
	imageMaxEdge = 2048
	imageQuality = 85

	statusRejected = "rejected"
	statusReady    = "ready"
)

var ErrPermanent = errors.New("permanement failure")

type Worker struct {
	db      *sql.DB
	storage *storage.Client
	logger  *slog.Logger
}

func permanentF(format string, args ...any) error {
	return fmt.Errorf("%w : %s", ErrPermanent, fmt.Sprintf(format, args...))
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
		isEmpty, err := w.claimAndProcess(ctx)
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
		id       int64
		kind     string
		payload  []byte
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

	if errors.Is(err, sql.ErrNoRows) {
		w.logger.Info("Queue is empty", "retry", pollInterval)
		return true, nil
	}
	if err != nil {
		return false, err
	}
	w.logger.Info("job processing started", "job id", id, "kind", kind, "attempts", attempts)
	if err := w.process(ctx, id, kind, payload); err != nil {
		if errors.Is(err, ErrPermanent) || attempts >= maxAttempts {
			w.logger.Error("Worker;ClaimAndProcess:Job Encountered Permanent error or reached maxAttempt")
			w.deadLetter(ctx, int(id), kind, payload)
			return false, err
		}
		if _, err := w.db.ExecContext(ctx, `UPDATE jobs SET last_error = $1 WHERE id = $2`, err.Error(), id); err != nil {
			w.logger.Error("Worker;claimAndProcess: Recording job failed")
		}
		return false, err
	}
	w.logger.Info("Worker:claimAndProcess;Job processed successfully")
	return false, nil
}

func (w *Worker) process(ctx context.Context, jobId int64, kind string, payload []byte) error {
	switch kind {
	case jobs.KindProcessListingImage:
		var pload jobs.ProcessImagePayload
		if err := json.Unmarshal(payload, &pload); err != nil {
			return permanentF("Unmarshall payload %v", err)
		}
		return w.processImages(ctx, jobId, pload)
	default:
		return permanentF("unknown job kind %q", kind)
	}
}

func (w *Worker) processImages(ctx context.Context, jobId int64, payload jobs.ProcessImagePayload) error {
	log := w.logger.With("job_id", jobId, "listing_id", payload.ListingId)
	normalizedImages := make([][]byte, 0, len(payload.Sources))
	for i, s := range payload.Sources {
		raw, err := w.storage.Get(ctx, s.Uploadkey)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				log.Error("Upload gone or never uploaded", "upload_key", s.Uploadkey)
				return w.finish(ctx, jobId, payload.ListingId, statusRejected)
			}
			log.Error("Worker:processimages:GetObject: Failed to fetch object")
			return fmt.Errorf("Get upload %q ; %w", s.Uploadkey, err)
		}
		b, err := io.ReadAll(raw)
		raw.Close()
		if err != nil {
			log.Error("Worker:processimages:ReadAll: Failed to fetch object")
			return fmt.Errorf("Read All %q ; %w", payload.Sources[i].Uploadkey, err)
		}

		out, err := imaging.Normalize(bytes.NewReader(b), imageMaxEdge, imageQuality)
		if err != nil {
			return permanentF("ProcessImages:imageing.Normalize %q : %v", s.Uploadkey, err)
		}
		normalizedImages = append(normalizedImages, out)
	}
	for i, img := range normalizedImages {
		key := payload.Sources[i].ObjectKey
		if err := w.storage.Put(ctx, key, bytes.NewReader(img)); err != nil {
			return fmt.Errorf("ProcessImages:imageing.Put %q : %v", key, err)
		}
	}
	return w.finish(ctx, jobId, payload.ListingId, statusReady)
}

func (w *Worker) finish(ctx context.Context, jobId int64, listingId uuid.UUID, status string) error {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("Worker:finish;Begin Transaction error %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if _, err := tx.ExecContext(ctx, `UPDATE listings SET status = $1 WHERE id = $2`, status, listingId); err != nil {
		return fmt.Errorf("Worker:finish;Update listing %q failed status %q error %w", listingId, status, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE id = $1`, jobId); err != nil {
		return fmt.Errorf("Worker:finish;Delete job %d failed status %q error %w", jobId, status, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("Worker:finish;Commit Transaction error %w", err)
	}
	return nil
}

func (w *Worker) deadLetter(ctx context.Context, jobId int, kind string, payload []byte) {
	log := w.logger.With("jobId", jobId, "kind", kind)
	//todo make a job and put it into job table with dead letter type
	listingId, found := w.listingIdFor(kind, payload)
	if !found {
		log.Error("Worker: deadLetter: Job dead letter, No resolveable listing")
		if _, err := w.db.ExecContext(ctx, `DELETE FROM jobs WHERE id = $1`, jobId); err != nil {
			log.Error("Worker:Deadletter: Deleting deadjob failed")
		}
		return
	}
	if err := w.finish(ctx, int64(jobId), listingId, statusRejected); err != nil {
		log.Error("Worker:Deadletter:Job failed to marked as finished")
	}
}

func (w *Worker) listingIdFor(kind string, payload []byte) (uuid.UUID, bool) {

	switch kind {
	case jobs.KindProcessListingImage:
		var p jobs.ProcessImagePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return uuid.Nil, false
		}
		return p.ListingId, true
	default:
		return uuid.Nil, false
	}

}
