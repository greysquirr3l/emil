package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"emil/internal/config"
	"emil/internal/converter"
	"emil/internal/manager"
	"emil/internal/security"
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
	testMode := flag.Bool("test", false, "Test mode - convert only the first EML file found and exit")

	// Add attachment options
	saveAttachments := flag.Bool("attachments", true, "Save email attachments")
	attachmentDir := flag.String("attachment-dir", "", "Directory for saving attachments (default: alongside PDFs)")

	// Add security options
	scanAttachments := flag.Bool("scan", false, "Scan attachments for viruses using ClamAV")
	clamdAddress := flag.String("clamd", "localhost:3310", "ClamAV daemon address")

	flag.Parse()

	// Create configuration
	cfg := &config.Config{
		SourceDir:       *srcDir,
		WorkerCount:     *workerCount,
		Verbose:         *verbose,
		RecursiveScan:   *recursive,
		MaxMemoryPct:    *maxMemPct,
		SaveAttachments: *saveAttachments,
		AttachmentDir:   *attachmentDir,
		ScanAttachments: *scanAttachments,
		ClamdAddress:    *clamdAddress,
	}

	// Print initial information
	fmt.Printf("Emil EML to PDF Converter\n")

	// Initialize security scanner if needed
	var scanner *security.Scanner
	if cfg.ScanAttachments {
		var err error
		scanner, err = security.NewScanner(true, cfg.ClamdAddress)
		if err != nil {
			log.Printf("Warning: Failed to initialize virus scanner: %v", err)
			log.Printf("Continuing without virus scanning")
			scanner = nil
			cfg.ScanAttachments = false
		} else if cfg.Verbose {
			fmt.Println("Virus scanning enabled")
		}
	}

	if *testMode {
		fmt.Println("Running in TEST MODE - will convert only the first EML file found")
		if err := runTestMode(*srcDir, *recursive, cfg, scanner); err != nil {
			log.Fatalf("Test failed: %v", err)
		}
		return
	}

	fmt.Printf("Scanning directory: %s\n", cfg.SourceDir)
	fmt.Printf("Workers: %d (auto-scaling enabled)\n", cfg.WorkerCount)
	fmt.Printf("Memory limit: %d%%\n", cfg.MaxMemoryPct)
	fmt.Printf("Attachment handling: %v\n", cfg.SaveAttachments)
	fmt.Printf("Virus scanning: %v\n", cfg.ScanAttachments)

	// Enable diagnostic monitor if requested
	if *diagnose {
		stopDiagnostics := util.StartDiagnosticMonitor(startTime, 30*time.Second, true)
		defer close(stopDiagnostics)
		util.LogFullDiagnostics(startTime)
	}

	// Create and start the manager
	mgr := manager.NewManager(cfg, scanner)

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

// runTestMode finds the first EML file and converts it
func runTestMode(dir string, recursive bool, cfg *config.Config, scanner *security.Scanner) error {
	fmt.Printf("Looking for EML files in %s\n", dir)

	var firstEMLFile string

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories if not recursive
		if info.IsDir() && !recursive && path != dir {
			return filepath.SkipDir
		}

		// Check if file is an EML file
		if !info.IsDir() && strings.ToLower(filepath.Ext(path)) == ".eml" {
			firstEMLFile = path
			return filepath.SkipDir // Stop after finding the first one
		}

		return nil
	}

	err := filepath.Walk(dir, walkFn)
	if err != nil {
		return fmt.Errorf("error scanning directory: %w", err)
	}

	if firstEMLFile == "" {
		return fmt.Errorf("no EML files found in %s", dir)
	}

	fmt.Printf("Found EML file: %s\n", firstEMLFile)
	fmt.Printf("Converting to PDF...\n")

	startTime := time.Now()
	result, err := converter.ConvertEMLToPDF(firstEMLFile, cfg, scanner)
	elapsed := time.Since(startTime).Round(time.Millisecond)

	if err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	absPath, err := filepath.Abs(result.OutputPath)
	if err != nil {
		absPath = result.OutputPath // Fall back to relative path if absolute path can't be determined
	}

	fmt.Printf("Conversion successful in %s\n", elapsed)
	fmt.Printf("PDF saved to: %s\n", absPath)

	// Check if the file exists and get its size
	info, err := os.Stat(result.OutputPath)
	if err != nil {
		return fmt.Errorf("error checking PDF file: %w", err)
	}

	fmt.Printf("PDF file size: %s\n", formatBytes(info.Size()))

	// Display attachment information if available
	if len(result.Attachments) > 0 {
		fmt.Printf("\nAttachments saved to: %s\n", filepath.Dir(result.Attachments[0].SavedPath))
		fmt.Printf("Attachments found: %d\n", len(result.Attachments))

		for i, att := range result.Attachments {
			fmt.Printf("  %d. %s (%s)", i+1, att.Filename, formatBytes(att.Size))

			if att.ScanResult != nil && att.ScanResult.Infected {
				fmt.Printf(" - SECURITY ALERT: Malware detected!")
			}
			fmt.Println()
		}
	}

	// Display security alerts if any
	if len(result.SecurityAlerts) > 0 {
		fmt.Printf("\nSecurity alerts: %d\n", len(result.SecurityAlerts))
		for _, alert := range result.SecurityAlerts {
			fmt.Printf("  - %s\n", alert)
		}
	}

	return nil
}

// formatBytes returns a human-readable byte string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
