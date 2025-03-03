package worker

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"emil/internal/converter"
	"emil/internal/models"
)

// Constants for worker behavior
const (
	maxConsecutiveFailures = 5    // Maximum number of consecutive failures before self-healing
	maxRetries             = 3    // Maximum retries per task
	backoffBase            = 500  // Base backoff in milliseconds
	heartbeatInterval      = 5000 // Milliseconds between worker heartbeats
)

// Worker represents a conversion worker
type Worker struct {
	id                int
	taskChan          <-chan models.Task
	statusChan        chan<- models.StatusUpdate
	done              chan struct{}
	failCount         int
	consecutiveErrors int
	maxRetries        int
	wg                sync.WaitGroup
	stopChan          chan struct{}
	verbose           bool
	lastActivity      time.Time
}

// NewWorker creates a new worker
func NewWorker(id int, taskChan <-chan models.Task, statusChan chan<- models.StatusUpdate, verbose bool) *Worker {
	return &Worker{
		id:           id,
		taskChan:     taskChan,
		statusChan:   statusChan,
		done:         make(chan struct{}),
		maxRetries:   maxRetries,
		stopChan:     make(chan struct{}),
		verbose:      verbose,
		lastActivity: time.Now(),
	}
}

// Start begins the worker's processing loop
func (w *Worker) Start(ctx context.Context, pauseChan <-chan bool) {
	w.wg.Add(1)

	go func() {
		defer w.wg.Done()
		defer close(w.done)

		// Start heartbeat monitoring
		heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
		defer cancelHeartbeat()
		go w.heartbeat(heartbeatCtx)

		for {
			select {
			case <-ctx.Done():
				return

			case <-w.stopChan:
				if w.verbose {
					log.Printf("Worker %d stopping on request", w.id)
				}
				return

			case pause := <-pauseChan:
				if pause {
					if w.verbose {
						log.Printf("Worker %d pausing due to resource constraints", w.id)
					}
					// Wait for unpause signal or context cancellation
					select {
					case <-pauseChan:
						if w.verbose {
							log.Printf("Worker %d resuming", w.id)
						}
					case <-ctx.Done():
						return
					case <-w.stopChan:
						if w.verbose {
							log.Printf("Worker %d stopping while paused", w.id)
						}
						return
					}
				}

			case task, ok := <-w.taskChan:
				if !ok {
					// Channel closed, no more tasks
					return
				}
				w.processTask(ctx, task)

				// Update last activity time
				w.lastActivity = time.Now()

				// Self-healing: If worker has too many consecutive failures, restart it
				if w.consecutiveErrors > maxConsecutiveFailures {
					if w.verbose {
						log.Printf("Worker %d self-healing after %d consecutive failures",
							w.id, w.consecutiveErrors)
					}
					// Reset error counters
					w.consecutiveErrors = 0
					w.failCount = 0

					// Force garbage collection
					debug.FreeOSMemory()
				}
			}
		}
	}()
}

// Done returns a channel that is closed when the worker completes
func (w *Worker) Done() <-chan struct{} {
	return w.done
}

// Stop requests the worker to stop
func (w *Worker) Stop() {
	close(w.stopChan)
}

// heartbeat periodically checks worker health
func (w *Worker) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(heartbeatInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check for worker unresponsiveness
			if time.Since(w.lastActivity) > time.Duration(heartbeatInterval*3)*time.Millisecond {
				if w.verbose {
					log.Printf("Worker %d detected as unresponsive, attempting recovery", w.id)
				}
				w.Stop()
			}
		}
	}
}

// processTask handles a single conversion task with retries
func (w *Worker) processTask(ctx context.Context, task models.Task) {
	// Initialize processing stats
	stats := models.ProcessingStats{
		StartTime: time.Now(),
		FileSize:  task.FileSize,
		WorkerID:  w.id,
	}

	// Update status to processing
	w.sendStatus(task.ID, models.StatusProcessing, 0, "Started processing", stats, nil)

	var err error
	var retries int

	for retries <= w.maxRetries {
		// Handle context cancellation
		select {
		case <-ctx.Done():
			stats.EndTime = time.Now()
			stats.Duration = stats.EndTime.Sub(stats.StartTime)
			stats.Retries = retries
			w.sendStatus(task.ID, models.StatusFailed, 0, "Cancelled", stats, ctx.Err())
			return
		default:
			// Continue processing
		}

		// Attempt conversion
		startConvert := time.Now()
		err = w.convertFile(ctx, task)
		conversionTime := time.Since(startConvert)

		if err == nil {
			// Success!
			stats.EndTime = time.Now()
			stats.Duration = stats.EndTime.Sub(stats.StartTime)
			stats.Retries = retries
			w.sendStatus(task.ID, models.StatusComplete, 1.0,
				fmt.Sprintf("Conversion complete in %s", conversionTime.Round(time.Millisecond)),
				stats, nil)

			w.failCount = 0         // Reset fail count on success
			w.consecutiveErrors = 0 // Reset consecutive errors
			return
		}

		// Handle failure with retries
		retries++
		w.failCount++
		w.consecutiveErrors++

		if retries <= w.maxRetries {
			backoff := time.Duration(retries*backoffBase) * time.Millisecond

			stats.Retries = retries
			w.sendStatus(task.ID, models.StatusProcessing, 0,
				fmt.Sprintf("Retrying (%d/%d) after %v: %v", retries, w.maxRetries, backoff, err),
				stats, nil)

			// Wait before retrying
			select {
			case <-ctx.Done():
				stats.EndTime = time.Now()
				stats.Duration = stats.EndTime.Sub(stats.StartTime)
				w.sendStatus(task.ID, models.StatusFailed, 0, "Cancelled during retry", stats, ctx.Err())
				return
			case <-time.After(backoff):
				// Continue to retry
			}
		}
	}

	// All retries failed
	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)
	stats.Retries = retries
	w.sendStatus(task.ID, models.StatusFailed, 0, "All retries failed", stats, err)
}

// convertFile performs the EML to PDF conversion
func (w *Worker) convertFile(ctx context.Context, task models.Task) error {
	// Create intermediate status updates to show progress
	w.sendStatus(task.ID, models.StatusProcessing, 0.25,
		"Reading EML file", models.ProcessingStats{}, nil)

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue processing
	}

	// Perform the actual conversion
	err := converter.ConvertEMLToPDF(task.FilePath)

	// Check for context cancellation again
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Continue
	}

	// Report 75% progress after conversion
	if err == nil {
		w.sendStatus(task.ID, models.StatusProcessing, 0.75,
			"PDF created, finalizing", models.ProcessingStats{}, nil)
	}

	return err
}

// sendStatus sends a status update to the manager
func (w *Worker) sendStatus(taskID string, status models.TaskStatus, progress float64,
	message string, stats models.ProcessingStats, err error) {

	update := models.StatusUpdate{
		WorkerID:        w.id,
		TaskID:          taskID,
		Status:          status,
		Progress:        progress,
		Message:         message,
		ProcessingStats: stats,
	}

	if err != nil {
		update.Error = err
	}

	select {
	case w.statusChan <- update:
		// Status sent successfully
	default:
		// Channel is full, log this issue
		if w.verbose {
			log.Printf("Worker %d: Status channel full, update dropped for task %s", w.id, taskID)
		}
	}
}
