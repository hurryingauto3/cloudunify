package sync

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"cloudunify/internal/database"
)

// Job represents a sync job with its current state
type Job struct {
	Item      *database.SyncQueueItem
	StartTime time.Time
	Progress  int
}

// Queue manages the sync queue operations
type Queue struct {
	db        *database.DB
	mu        sync.RWMutex
	active    map[int64]*Job
	paused    bool
	listeners []func(event QueueEvent)
}

// QueueEvent represents an event that occurred in the queue
type QueueEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
	Time    time.Time   `json:"time"`
}

// Event types
const (
	EventJobStarted   = "job_started"
	EventJobProgress  = "job_progress"
	EventJobCompleted = "job_completed"
	EventJobFailed    = "job_failed"
)

// NewQueue creates a new sync queue
func NewQueue(db *database.DB) *Queue {
	return &Queue{
		db:     db,
		active: make(map[int64]*Job),
	}
}

// Enqueue adds a new item to the sync queue
func (q *Queue) Enqueue(ctx context.Context, operation database.SyncOperation, virtualPath string, localPath string, providerID *int64, priority int) (*database.SyncQueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item := &database.SyncQueueItem{
		Operation:   operation,
		VirtualPath: virtualPath,
		LocalPath:   localPath,
		Priority:    priority,
		Status:      database.SyncStatusPending,
	}

	if providerID != nil {
		item.ProviderID = sql.NullInt64{Int64: *providerID, Valid: true}
	}

	if err := q.db.EnqueueSync(ctx, item); err != nil {
		return nil, fmt.Errorf("failed to enqueue sync: %w", err)
	}

	return item, nil
}

// Dequeue gets the next pending item from the queue
func (q *Queue) Dequeue(ctx context.Context) (*database.SyncQueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.paused {
		return nil, nil
	}

	item, err := q.db.DequeueSync(ctx)
	if err != nil {
		return nil, err
	}

	if item != nil {
		q.active[item.ID] = &Job{
			Item:      item,
			StartTime: time.Now(),
		}
		q.emit(QueueEvent{
			Type:    EventJobStarted,
			Payload: item,
			Time:    time.Now(),
		})
	}

	return item, nil
}

// UpdateProgress updates the progress of an active job
func (q *Queue) UpdateProgress(ctx context.Context, id int64, progress int) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if job, ok := q.active[id]; ok {
		job.Progress = progress
	}

	if err := q.db.UpdateSyncProgress(ctx, id, progress); err != nil {
		return err
	}

	q.emit(QueueEvent{
		Type: EventJobProgress,
		Payload: map[string]interface{}{
			"id":       id,
			"progress": progress,
		},
		Time: time.Now(),
	})

	return nil
}

// Complete marks a job as completed
func (q *Queue) Complete(ctx context.Context, id int64) error {
	q.mu.Lock()
	delete(q.active, id)
	q.mu.Unlock()

	if err := q.db.CompleteSyncItem(ctx, id); err != nil {
		return err
	}

	q.emit(QueueEvent{
		Type:    EventJobCompleted,
		Payload: map[string]interface{}{"id": id},
		Time:    time.Now(),
	})

	return nil
}

// Fail marks a job as failed
func (q *Queue) Fail(ctx context.Context, id int64, errorMsg string) error {
	q.mu.Lock()
	delete(q.active, id)
	q.mu.Unlock()

	if err := q.db.FailSyncItem(ctx, id, errorMsg); err != nil {
		return err
	}

	q.emit(QueueEvent{
		Type: EventJobFailed,
		Payload: map[string]interface{}{
			"id":    id,
			"error": errorMsg,
		},
		Time: time.Now(),
	})

	return nil
}

// Retry requeues a failed job
func (q *Queue) Retry(ctx context.Context, id int64) error {
	return q.db.RetryFailedSync(ctx, id)
}

// Cancel removes a pending job from the queue
func (q *Queue) Cancel(ctx context.Context, id int64) error {
	q.mu.Lock()
	delete(q.active, id)
	q.mu.Unlock()

	return q.db.DeleteSyncItem(ctx, id)
}

// Pause pauses queue processing
func (q *Queue) Pause() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = true
}

// Resume resumes queue processing
func (q *Queue) Resume() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = false
}

// IsPaused returns whether the queue is paused
func (q *Queue) IsPaused() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.paused
}

// List returns items in the queue with optional status filter
func (q *Queue) List(ctx context.Context, status database.SyncStatus) ([]*database.SyncQueueItem, error) {
	return q.db.ListSyncQueue(ctx, status)
}

// GetActive returns currently active jobs
func (q *Queue) GetActive() []*Job {
	q.mu.RLock()
	defer q.mu.RUnlock()

	jobs := make([]*Job, 0, len(q.active))
	for _, job := range q.active {
		jobs = append(jobs, job)
	}
	return jobs
}

// ClearCompleted removes all completed items from the queue
func (q *Queue) ClearCompleted(ctx context.Context) error {
	return q.db.ClearCompletedSync(ctx)
}

// OnEvent registers a listener for queue events
func (q *Queue) OnEvent(listener func(event QueueEvent)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.listeners = append(q.listeners, listener)
}

// emit sends an event to all listeners
func (q *Queue) emit(event QueueEvent) {
	for _, listener := range q.listeners {
		go listener(event)
	}
}

// Stats returns queue statistics
func (q *Queue) Stats(ctx context.Context) (*QueueStats, error) {
	pending, err := q.db.ListSyncQueue(ctx, database.SyncStatusPending)
	if err != nil {
		return nil, err
	}

	processing, err := q.db.ListSyncQueue(ctx, database.SyncStatusProcessing)
	if err != nil {
		return nil, err
	}

	failed, err := q.db.ListSyncQueue(ctx, database.SyncStatusFailed)
	if err != nil {
		return nil, err
	}

	return &QueueStats{
		Pending:    len(pending),
		Processing: len(processing),
		Failed:     len(failed),
		Paused:     q.IsPaused(),
	}, nil
}

// QueueStats contains queue statistics
type QueueStats struct {
	Pending    int  `json:"pending"`
	Processing int  `json:"processing"`
	Failed     int  `json:"failed"`
	Paused     bool `json:"paused"`
}
