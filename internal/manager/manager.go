package manager

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"

	"emil/internal/config"
	"emil/internal/models"
	"emil/internal/resource"
	"emil/internal/security"
	"emil/internal/worker"
)

const (
	// Time between progress updates when verbose mode is on
	verboseUpdateInterval = 5 * time.Second

	// How long before considering a task stuck
	stuckTaskThreshold = 3 * time.Minute
)

// Manager handles task discovery and distribution
type Manager struct {
	config        *config.Config
	workers       []*worker.Worker
	taskChan      chan models.Task
	statusChan    chan models.StatusUpdate
	statsLock     sync.RWMutex
	stats         models.Stats
	cancel        context.CancelFunc
	progressBar   *progressbar.ProgressBar
	tasksByID     map[string]models.Task
	tasksByIDLock sync.RWMutex
	resourceMgr   *resource.Manager
	failedTasks   []models.Task
	stuckTasks    map[string]time.Time
	stuckTaskLock sync.Mutex
	scanner       *security.Scanner
}

// NewManager creates a new manager instance
func NewManager(cfg *config.Config, scanner *security.Scanner) *Manager {
	return &Manager{
		config:     cfg,
		taskChan:   make(chan models.Task, 100),
		statusChan: make(chan models.StatusUpdate, 100),
		tasksByID:  make(map[string]models.Task),
		stats: models.Stats{
			StartTime:      time.Now(),
			CurrentWorkers: cfg.WorkerCount,
			MaxWorkers:     cfg.WorkerCount * 2,
			MinWorkers:     1,
		},
		stuckTasks: make(map[string]time.Time),
		scanner:    scanner,
	}
}

// Start begins the processing operation
func (m *Manager) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// Initialize resource manager with config parameters
	m.resourceMgr = resource.NewManager(
		1,                              // Min workers
		m.config.WorkerCount*2,         // Max workers
		float64(m.config.MaxMemoryPct), // Use config's memory percentage
		80.0,                           // Target CPU percentage
		m.config.Verbose,               // Verbose logging
	)
	m.resourceMgr.Start(ctx)

	// Start monitoring for stuck tasks
	go m.monitorStuckTasks(ctx)

	// Discover files first to get total count
	files, err := m.discoverFiles()
	if err != nil {
		return fmt.Errorf("file discovery failed: %w", err)
	}

	m.statsLock.Lock()
	m.stats.Discovered = len(files)
	var totalSize int64
	for _, fileInfo := range files {
		totalSize += fileInfo.Size
	}
	m.stats.TotalFileSize = totalSize
	m.statsLock.Unlock()

	fmt.Printf("Found %d EML files to process (%.2f MB total)\n",
		len(files), float64(totalSize)/(1024*1024))

	// Create progress bar
	m.progressBar = progressbar.NewOptions(len(files),
		progressbar.OptionSetDescription("Converting"),
		progressbar.OptionShowCount(),
		progressbar.OptionShowElapsedTimeOnFinish(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// Start workers
	m.initWorkers(ctx)

	// Start status monitor
	go m.monitorStatus(ctx)

	// If verbose, show more detailed progress updates
	if m.config.Verbose {
		go m.verboseProgressUpdates(ctx)
	}

	// Enqueue tasks
	for _, fileInfo := range files {
		task := models.Task{
			ID:        filepath.Base(fileInfo.Path),
			FilePath:  fileInfo.Path,
			Status:    models.StatusPending,
			FileSize:  fileInfo.Size,
			StartTime: time.Now(),
		}

		m.tasksByIDLock.Lock()
		m.tasksByID[task.ID] = task
		m.tasksByIDLock.Unlock()

		m.taskChan <- task
	}

	// Wait for all tasks to be processed
	close(m.taskChan)

	// Wait for workers to finish
	for _, w := range m.workers {
		<-w.Done()
	}

	m.statsLock.Lock()
	m.stats.EndTime = time.Now()
	m.statsLock.Unlock()

	// Show remaining failed tasks if any
	if len(m.failedTasks) > 0 {
		fmt.Printf("\nFailed to process %d files:\n", len(m.failedTasks))
		for i, task := range m.failedTasks {
			if i < 10 { // Limit to first 10
				fmt.Printf("  - %s: %v\n", task.FilePath, task.Error)
			} else {
				fmt.Printf("  - ... and %d more\n", len(m.failedTasks)-10)
				break
			}
		}
	}

	return nil
}

// Stop gracefully shuts down processing
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Stats returns current statistics
func (m *Manager) Stats() models.Stats {
	m.statsLock.RLock()
	defer m.statsLock.RUnlock()
	return m.stats
}

// FileInfo represents a discovered file
type FileInfo struct {
	Path string
	Size int64
}

// discoverFiles finds all EML files in the source directory
func (m *Manager) discoverFiles() ([]FileInfo, error) {
	var files []FileInfo

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories if not recursive
		if info.IsDir() && !m.config.RecursiveScan && path != m.config.SourceDir {
			return filepath.SkipDir
		}

		// Check if file is an EML file
		if !info.IsDir() && strings.ToLower(filepath.Ext(path)) == ".eml" {
			files = append(files, FileInfo{
				Path: path,
				Size: info.Size(),
			})
		}

		return nil
	}

	if err := filepath.Walk(m.config.SourceDir, walkFn); err != nil {
		return nil, err
	}

	return files, nil
}

// initWorkers creates and starts the worker pool
func (m *Manager) initWorkers(ctx context.Context) {
	m.workers = make([]*worker.Worker, m.config.WorkerCount)

	for i := 0; i < m.config.WorkerCount; i++ {
		m.workers[i] = worker.NewWorker(i, m.taskChan, m.statusChan, m.config, m.scanner)
		m.workers[i].Start(ctx, m.resourceMgr.PauseControl())
	}

	// Start goroutine to handle dynamic worker scaling
	go func() {
		workerPool := make(map[int]*worker.Worker)
		nextWorkerID := m.config.WorkerCount

		// Initialize with current workers
		for i, w := range m.workers {
			workerPool[i] = w
		}

		for {
			select {
			case <-ctx.Done():
				return

			case adjustment := <-m.resourceMgr.WorkerControl():
				if adjustment > 0 {
					// Add a worker
					w := worker.NewWorker(nextWorkerID, m.taskChan, m.statusChan, m.config, m.scanner)
					w.Start(ctx, m.resourceMgr.PauseControl())
					workerPool[nextWorkerID] = w
					nextWorkerID++

					m.statsLock.Lock()
					m.stats.CurrentWorkers++
					if m.stats.CurrentWorkers > m.stats.MaxWorkers {
						m.stats.MaxWorkers = m.stats.CurrentWorkers
					}
					m.statsLock.Unlock()

				} else if adjustment < 0 {
					// Remove a worker - find the highest ID
					if len(workerPool) > 1 { // Always keep at least one worker
						highestID := -1
						for id := range workerPool {
							if id > highestID {
								highestID = id
							}
						}

						if highestID >= 0 {
							workerPool[highestID].Stop()
							delete(workerPool, highestID)

							m.statsLock.Lock()
							m.stats.CurrentWorkers--
							if m.stats.CurrentWorkers < m.stats.MinWorkers {
								m.stats.MinWorkers = m.stats.CurrentWorkers
							}
							m.statsLock.Unlock()
						}
					}
				}
			}
		}
	}()
}

// monitorStatus processes status updates from workers
func (m *Manager) monitorStatus(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-m.statusChan:
			m.handleStatusUpdate(update)
		}
	}
}

// handleStatusUpdate processes a worker status update
func (m *Manager) handleStatusUpdate(update models.StatusUpdate) {
	m.tasksByIDLock.Lock()
	if task, exists := m.tasksByID[update.TaskID]; exists {
		task.Status = update.Status
		task.Error = update.Error

		// Mark task as no longer stuck if it's being processed
		if update.Status == models.StatusProcessing {
			m.stuckTaskLock.Lock()
			m.stuckTasks[update.TaskID] = time.Now()
			m.stuckTaskLock.Unlock()
		} else {
			m.stuckTaskLock.Lock()
			delete(m.stuckTasks, update.TaskID)
			m.stuckTaskLock.Unlock()
		}

		// Update task completion time
		if update.Status == models.StatusComplete || update.Status == models.StatusFailed {
			task.CompleteTime = time.Now()
			task.Retries = update.ProcessingStats.Retries
		}

		m.tasksByID[update.TaskID] = task
	}
	m.tasksByIDLock.Unlock()

	m.statsLock.Lock()
	switch update.Status {
	case models.StatusProcessing:
		m.stats.Processing++
	case models.StatusComplete:
		m.stats.Processed++
		m.stats.Successful++
		m.stats.Processing--
		m.progressBar.Add(1)

		// Update speed calculation
		duration := update.ProcessingStats.Duration.Seconds()
		if duration > 0 && update.ProcessingStats.FileSize > 0 {
			speed := float64(update.ProcessingStats.FileSize) / duration
			// Weighted average to smooth out the speed
			if m.stats.AverageSpeed == 0 {
				m.stats.AverageSpeed = speed
			} else {
				m.stats.AverageSpeed = (m.stats.AverageSpeed * 0.7) + (speed * 0.3)
			}
		}

	case models.StatusFailed:
		m.stats.Processed++
		m.stats.Failed++
		m.stats.Processing--
		m.progressBar.Add(1)

		// Store failed task for final report
		m.tasksByIDLock.Lock()
		if task, exists := m.tasksByID[update.TaskID]; exists {
			m.failedTasks = append(m.failedTasks, task)
		}
		m.tasksByIDLock.Unlock()

		if m.config.Verbose {
			fmt.Printf("\nFailed to convert %s: %v\n", update.TaskID, update.Error)
		}
	}
	m.statsLock.Unlock()
}

// verboseProgressUpdates shows detailed progress in verbose mode
func (m *Manager) verboseProgressUpdates(ctx context.Context) {
	ticker := time.NewTicker(verboseUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			m.statsLock.RLock()
			stats := m.stats
			m.statsLock.RUnlock()

			elapsed := time.Since(stats.StartTime).Seconds()
			var bytesPerSec float64
			if stats.AverageSpeed > 0 {
				bytesPerSec = stats.AverageSpeed
			} else if elapsed > 0 {
				bytesPerSec = float64(stats.TotalFileSize) / elapsed
			}

			remainingFiles := stats.Discovered - stats.Processed
			var estRemaining time.Duration
			if bytesPerSec > 0 && stats.Processed > 0 {
				avgFileSize := float64(stats.TotalFileSize) / float64(stats.Discovered)
				estRemaining = time.Duration(float64(remainingFiles)*avgFileSize/bytesPerSec) * time.Second
			} else {
				estRemaining = time.Duration(0)
			}

			memUsage := m.resourceMgr.MemoryUsage()

			fmt.Printf("\nStatus: %d/%d files processed (%.1f%%) | Workers: %d | Memory: %.1f%% | Speed: %.2f KB/s | ETA: %s\n",
				stats.Processed, stats.Discovered,
				float64(stats.Processed)/float64(stats.Discovered)*100,
				stats.CurrentWorkers,
				memUsage,
				bytesPerSec/1024,
				estRemaining.Round(time.Second).String())
		}
	}
}

// monitorStuckTasks checks for tasks that appear to be stuck
func (m *Manager) monitorStuckTasks(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			now := time.Now()
			m.stuckTaskLock.Lock()

			for taskID, startTime := range m.stuckTasks {
				if now.Sub(startTime) > stuckTaskThreshold {
					log.Printf("WARNING: Task %s appears to be stuck (processing for %s)",
						taskID, now.Sub(startTime).Round(time.Second))

					// Reset the timer so we don't warn constantly
					m.stuckTasks[taskID] = now.Add(-stuckTaskThreshold / 2)
				}
			}

			m.stuckTaskLock.Unlock()
		}
	}
}
