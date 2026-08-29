package domain

import "time"

// JobStatus represents the lifecycle state of a background job.
type JobStatus string

const (
	JobScheduled   JobStatus = "scheduled"
	JobPending     JobStatus = "pending"
	JobRunning     JobStatus = "running"
	JobDone        JobStatus = "done"
	JobSucceeded   JobStatus = "succeeded"
	JobFailed      JobStatus = "failed"
	JobCancelled   JobStatus = "cancelled"
	JobNetworkBusy JobStatus = "network_busy"
)

// Job represents a background task: upstream refresh, node test, score recalculation, etc.
type Job struct {
	ID         string
	Kind       string // e.g. "refresh_upstream", "test_node", "recalculate_scores"
	Status     JobStatus
	Progress   int    // 0–100
	EntityID   string // upstream ID, node ID, etc.
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time
}

// IsTerminal reports whether the job has reached a final state.
func (j *Job) IsTerminal() bool {
	return j.Status == JobDone || j.Status == JobSucceeded || j.Status == JobFailed || j.Status == JobCancelled
}
