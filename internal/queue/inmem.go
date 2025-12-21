package queue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// InMemQueue is an in-process job queue with worker pool and deduplication
type InMemQueue struct {
	jobs       chan Job
	inflight   map[string]bool // Track in-flight job keys for deduplication
	mu         sync.RWMutex
	wg         sync.WaitGroup
	numWorkers int
	logger     *slog.Logger
	stopOnce   sync.Once
}

// NewInMemQueue creates a new in-memory job queue
func NewInMemQueue(numWorkers int, logger *slog.Logger) *InMemQueue {
	return &InMemQueue{
		jobs:       make(chan Job, 100), // Buffered channel for 100 jobs
		inflight:   make(map[string]bool),
		numWorkers: numWorkers,
		logger:     logger,
	}
}

// Enqueue adds a job to the queue if not already in-flight
func (q *InMemQueue) Enqueue(ctx context.Context, job Job) error {
	q.logger.Info("attempting to enqueue job",
		"kind", job.Kind(),
		"key", job.Key(),
	)

	q.mu.Lock()
	defer q.mu.Unlock()

	key := job.Key()

	// Check if job is already in-flight (deduplication)
	if q.inflight[key] {
		q.logger.Warn("job already in-flight, skipping duplicate",
			"kind", job.Kind(),
			"key", key,
			"total_inflight", len(q.inflight),
		)
		return nil
	}

	// Mark as in-flight
	q.inflight[key] = true

	// Enqueue the job
	select {
	case q.jobs <- job:
		q.logger.Info("job enqueued successfully",
			"kind", job.Kind(),
			"key", key,
			"queue_size", len(q.jobs),
			"total_inflight", len(q.inflight),
		)
		return nil
	case <-ctx.Done():
		// Remove from inflight if we couldn't enqueue
		delete(q.inflight, key)
		q.logger.Error("failed to enqueue job - context cancelled",
			"kind", job.Kind(),
			"key", key,
			"error", ctx.Err(),
		)
		return ctx.Err()
	}
}

// Start begins processing jobs with the configured number of workers
func (q *InMemQueue) Start(ctx context.Context) error {
	q.logger.Info("starting job queue", "workers", q.numWorkers)

	for i := 0; i < q.numWorkers; i++ {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}

	return nil
}

// Stop gracefully shuts down the queue and waits for workers to finish
func (q *InMemQueue) Stop(ctx context.Context) error {
	var err error
	q.stopOnce.Do(func() {
		q.logger.Info("stopping job queue")
		close(q.jobs)
		q.wg.Wait()
		q.logger.Info("job queue stopped")
	})
	return err
}

// worker processes jobs from the queue
func (q *InMemQueue) worker(ctx context.Context, id int) {
	defer q.wg.Done()

	q.logger.Info("worker started", "worker_id", id)

	for job := range q.jobs {
		key := job.Key()

		q.logger.Info("worker processing job",
			"worker_id", id,
			"kind", job.Kind(),
			"key", key,
		)

		// Execute the job
		if err := job.Run(ctx); err != nil {
			q.logger.Error("job execution failed",
				"worker_id", id,
				"kind", job.Kind(),
				"key", key,
				"error", err,
			)
		} else {
			q.logger.Info("job completed successfully",
				"worker_id", id,
				"kind", job.Kind(),
				"key", key,
			)
		}

		// Remove from inflight tracking
		q.mu.Lock()
		delete(q.inflight, key)
		q.mu.Unlock()
	}

	q.logger.Info("worker stopped", "worker_id", id)
}

// InFlight returns the number of in-flight jobs (for debugging/monitoring)
func (q *InMemQueue) InFlight() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.inflight)
}

// IsInFlight checks if a job key is currently in-flight
func (q *InMemQueue) IsInFlight(key string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.inflight[key]
}

// String returns a string representation of the queue state
func (q *InMemQueue) String() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return fmt.Sprintf("InMemQueue{workers=%d, inflight=%d, queued=%d}",
		q.numWorkers, len(q.inflight), len(q.jobs))
}
