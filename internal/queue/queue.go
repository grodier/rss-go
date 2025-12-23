package queue

import "context"

// Job represents a unit of work that can be enqueued and executed
type Job interface {
	// Kind returns the job type identifier (e.g., "discovery")
	Kind() string

	// Key returns a unique key for deduplication
	// Jobs with the same key will not be queued twice
	Key() string

	// Run executes the job
	Run(ctx context.Context) error
}

// JobQueue defines the interface for a job queue implementation
// This abstraction allows swapping in-process queue for Kafka/SQS/RabbitMQ
type JobQueue interface {
	// Enqueue adds a job to the queue
	// Returns error if enqueue fails
	Enqueue(ctx context.Context, job Job) error

	// Start begins processing jobs
	// Should be called once during application startup
	Start(ctx context.Context) error

	// Stop gracefully shuts down the queue
	// Waits for in-flight jobs to complete
	Stop(ctx context.Context) error
}
