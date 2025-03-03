package models

import "time"

// TaskStatus represents the current status of a conversion task
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusProcessing TaskStatus = "processing"
	StatusComplete   TaskStatus = "complete"
	StatusFailed     TaskStatus = "failed"
)

// Task represents a conversion task from EML to PDF
type Task struct {
	ID           string
	FilePath     string
	Status       TaskStatus
	Error        error
	FileSize     int64
	StartTime    time.Time
	CompleteTime time.Time
	Retries      int
}

// StatusUpdate represents a message from a worker about task status
type StatusUpdate struct {
	WorkerID        int
	TaskID          string
	Status          TaskStatus
	Progress        float64
	Message         string
	Error           error
	ProcessingStats ProcessingStats
}

// ResourceStats tracks system resource usage
type ResourceStats struct {
	CPUUsage    float64
	MemoryUsage float64
	LastCheck   time.Time
}

// ProcessingStats tracks statistics for a single processing operation
type ProcessingStats struct {
	StartTime time.Time
	EndTime   time.Time
	FileSize  int64
	Duration  time.Duration
	WorkerID  int
	Retries   int
}

// Stats tracks overall job statistics
type Stats struct {
	Discovered     int
	Processing     int
	Processed      int
	Successful     int
	Failed         int
	StartTime      time.Time
	EndTime        time.Time
	TotalFileSize  int64
	AverageSpeed   float64 // bytes per second
	PeakMemoryUsed int64
	MaxWorkers     int
	MinWorkers     int
	CurrentWorkers int
}
