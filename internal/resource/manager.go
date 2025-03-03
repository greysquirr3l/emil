package resource

import (
	"context"
	"log"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const (
	// Memory management constants
	memoryLowWatermark  = 60 // Percentage of memory usage that's considered safe
	memoryHighWatermark = 75 // Percentage that triggers aggressive GC

	// Time between resource checks
	resourceCheckInterval = 500 * time.Millisecond
)

// Manager handles dynamic scaling of workers based on resource usage
type Manager struct {
	sync.Mutex
	targetMemory    float64
	targetCPU       float64
	maxWorkers      int
	minWorkers      int
	currentWorkers  int
	workerControl   chan int // +1 to add, -1 to remove worker
	pauseProcessing chan bool
	lastScaleDown   time.Time
	scaleUpDelay    time.Duration
	memUsage        float64
	verbose         bool
}

// NewManager creates a resource manager
func NewManager(minWorkers, maxWorkers int, targetMemory, targetCPU float64, verbose bool) *Manager {
	return &Manager{
		targetMemory:    targetMemory,
		targetCPU:       targetCPU,
		maxWorkers:      maxWorkers,
		minWorkers:      minWorkers,
		currentWorkers:  maxWorkers,
		workerControl:   make(chan int, maxWorkers*2),
		pauseProcessing: make(chan bool, 1),
		scaleUpDelay:    30 * time.Second,
		verbose:         verbose,
	}
}

// Start begins resource monitoring
func (rm *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(resourceCheckInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rm.adjustResources()
				rm.monitorMemory()
			}
		}
	}()
}

// WorkerControl returns the channel used to control workers
func (rm *Manager) WorkerControl() <-chan int {
	return rm.workerControl
}

// PauseControl returns the channel used to pause processing
func (rm *Manager) PauseControl() <-chan bool {
	return rm.pauseProcessing
}

// CurrentWorkers returns the current number of workers
func (rm *Manager) CurrentWorkers() int {
	rm.Lock()
	defer rm.Unlock()
	return rm.currentWorkers
}

// MemoryUsage returns the current memory usage percentage
func (rm *Manager) MemoryUsage() float64 {
	rm.Lock()
	defer rm.Unlock()
	return rm.memUsage
}

// ForceGC triggers garbage collection
func (rm *Manager) ForceGC() {
	debug.FreeOSMemory()
}

// adjustResources monitors and adjusts resource allocation
func (rm *Manager) adjustResources() {
	rm.Lock()
	defer rm.Unlock()

	// Get current memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsage := float64(m.Alloc) / float64(m.Sys) * 100
	rm.memUsage = memUsage

	// If memory usage is too high, force GC and pause processing
	if memUsage > rm.targetMemory {
		debug.FreeOSMemory()
		select {
		case rm.pauseProcessing <- true:
			if rm.verbose {
				log.Printf("Memory usage high (%.1f%%), pausing processing", memUsage)
			}
		default:
		}

		// Reduce worker count temporarily
		newWorkers := int(float64(rm.currentWorkers) * 0.75)
		if newWorkers < rm.minWorkers {
			newWorkers = rm.minWorkers
		}
		rm.adjustWorkerCount(newWorkers)
		return
	}

	// Resume processing if previously paused
	select {
	case <-rm.pauseProcessing:
		if rm.verbose {
			log.Printf("Resuming processing, memory usage: %.1f%%", memUsage)
		}
	default:
		// Already running
	}

	// Adjust worker count based on memory usage trend
	if memUsage > rm.targetMemory*0.9 {
		// Getting close to memory limit, reduce workers
		newWorkers := int(float64(rm.currentWorkers) * 0.9)
		if newWorkers < rm.minWorkers {
			newWorkers = rm.minWorkers
		}
		rm.adjustWorkerCount(newWorkers)
	} else if memUsage < rm.targetMemory*0.6 {
		// Memory usage is low, can increase workers if we haven't recently scaled down
		if time.Since(rm.lastScaleDown) > rm.scaleUpDelay {
			newWorkers := rm.currentWorkers + 1
			if newWorkers > rm.maxWorkers {
				newWorkers = rm.maxWorkers
			}
			rm.adjustWorkerCount(newWorkers)
		}
	}
}

// adjustWorkerCount changes the number of active workers
func (rm *Manager) adjustWorkerCount(newCount int) {
	if newCount == rm.currentWorkers {
		return
	}

	delta := newCount - rm.currentWorkers
	if delta > 0 {
		// Adding workers
		for i := 0; i < delta; i++ {
			rm.workerControl <- 1
		}
		if rm.verbose {
			log.Printf("Scaling up workers from %d to %d", rm.currentWorkers, newCount)
		}
	} else {
		// Removing workers
		for i := 0; i < -delta; i++ {
			rm.workerControl <- -1
		}
		rm.lastScaleDown = time.Now()
		if rm.verbose {
			log.Printf("Scaling down workers from %d to %d", rm.currentWorkers, newCount)
		}
	}
	rm.currentWorkers = newCount
}

// monitorMemory checks memory usage and takes action if needed
func (rm *Manager) monitorMemory() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsage := float64(m.Alloc) / float64(m.Sys) * 100

	// Update stored memory usage
	rm.Lock()
	rm.memUsage = memUsage
	rm.Unlock()

	if memUsage > memoryHighWatermark {
		// Memory usage is critical, take immediate action
		debug.FreeOSMemory()
		runtime.GC()

		// Force minimum workers
		rm.Lock()
		if rm.currentWorkers > rm.minWorkers {
			rm.adjustWorkerCount(rm.minWorkers)
			rm.lastScaleDown = time.Now()
		}
		rm.Unlock()

		// Wait for memory to be reclaimed
		time.Sleep(500 * time.Millisecond)

		// If memory is still high, pause processing
		if memUsage > memoryHighWatermark {
			select {
			case rm.pauseProcessing <- true:
				if rm.verbose {
					log.Printf("CRITICAL: Memory usage at %.1f%%, pausing all processing", memUsage)
				}
			default:
			}
		}
	}
}
