package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	"emil/internal/config"
	"emil/internal/manager"
	"emil/internal/util"
)

func main() {
	// Application start time
	startTime := time.Now()

	// Set GOMAXPROCS to use available cores efficiently
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Configure garbage collection for better performance
	debug.SetGCPercent(100) // Default is 100, lower means more aggressive GC

	// Parse command line flags
	srcDir := flag.String("src", ".", "Source directory to scan for EML files")
	workerCount := flag.Int("workers", runtime.NumCPU(), "Initial number of worker threads")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	recursive := flag.Bool("recursive", true, "Recursively scan directories")
	diagnose := flag.Bool("diagnose", false, "Show diagnostic information")
	maxMemPct := flag.Int("max-mem", 75, "Maximum memory usage percentage target")
	flag.Parse()

	// Create configuration
	cfg := &config.Config{
		SourceDir:     *srcDir,
		WorkerCount:   *workerCount,
		Verbose:       *verbose,
		RecursiveScan: *recursive,
		MaxMemoryPct:  *maxMemPct, // Use maxMemPct here
	}

	// Print initial information
	fmt.Printf("Emil EML to PDF Converter - by greysqurr3l\n")
	fmt.Printf("Scanning directory: %s\n", cfg.SourceDir)
	fmt.Printf("Workers: %d (auto-scaling enabled)\n", cfg.WorkerCount)
	fmt.Printf("Memory limit: %d%%\n", cfg.MaxMemoryPct)

	// Enable diagnostic monitor if requested
	if *diagnose {
		stopDiagnostics := util.StartDiagnosticMonitor(startTime, 30*time.Second, true)
		defer close(stopDiagnostics)
		util.LogFullDiagnostics(startTime)
	}

	// Create and start the manager
	mgr := manager.NewManager(cfg)

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
		mgr.Stop()

		// Log diagnostics before exit if enabled
		if *diagnose {
			util.LogFullDiagnostics(startTime)
		}
	}()

	// Start processing
	if err := mgr.Start(); err != nil {
		log.Fatalf("Error: %v", err)
	}

	// Get final stats
	stats := mgr.Stats()
	elapsed := time.Since(startTime).Round(time.Millisecond)

	// Calculate processing speed
	var filesPerSec float64
	if elapsed.Seconds() > 0 {
		filesPerSec = float64(stats.Processed) / elapsed.Seconds()
	}

	// Calculate data processing speed
	var mbPerSec float64
	if elapsed.Seconds() > 0 {
		mbPerSec = float64(stats.TotalFileSize) / elapsed.Seconds() / (1024 * 1024)
	}

	fmt.Printf("\nProcessing completed in %s\n", elapsed)
	fmt.Printf("Total files processed: %d (%.2f files/sec)\n", stats.Processed, filesPerSec)
	fmt.Printf("Data processed: %.2f MB (%.2f MB/sec)\n",
		float64(stats.TotalFileSize)/(1024*1024), mbPerSec)
	fmt.Printf("Successful: %d\n", stats.Successful)
	fmt.Printf("Failed: %d\n", stats.Failed)

	// Show worker scaling metrics
	fmt.Printf("Worker scaling: min=%d, max=%d\n", stats.MinWorkers, stats.MaxWorkers)

	// Log final diagnostics if enabled
	if *diagnose {
		util.LogFullDiagnostics(startTime)
	}
}
