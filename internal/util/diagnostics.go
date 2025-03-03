package util

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

// ProcessInfo holds information about the process
type ProcessInfo struct {
	PID         int
	Goroutines  int
	Memory      MemStats
	CPUCores    int
	GoVersion   string
	StartTime   time.Time
	ElapsedTime time.Duration
}

// MemStats holds memory statistics information
type MemStats struct {
	Alloc      string
	TotalAlloc string
	Sys        string
	NumGC      uint32
	HeapAlloc  string
	HeapSys    string
	HeapIdle   string
	HeapInUse  string
	StackInUse string
}

// GetProcessInfo returns diagnostic information about the running process
func GetProcessInfo(startTime time.Time) ProcessInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return ProcessInfo{
		PID:        os.Getpid(),
		Goroutines: runtime.NumGoroutine(),
		Memory: MemStats{
			Alloc:      formatBytes(m.Alloc),
			TotalAlloc: formatBytes(m.TotalAlloc),
			Sys:        formatBytes(m.Sys),
			NumGC:      m.NumGC,
			HeapAlloc:  formatBytes(m.HeapAlloc),
			HeapSys:    formatBytes(m.HeapSys),
			HeapIdle:   formatBytes(m.HeapIdle),
			HeapInUse:  formatBytes(m.HeapInuse),  // Fixed case: HeapInuse
			StackInUse: formatBytes(m.StackInuse), // Fixed case: StackInuse
		},
		CPUCores:    runtime.NumCPU(),
		GoVersion:   runtime.Version(),
		StartTime:   startTime,
		ElapsedTime: time.Since(startTime),
	}
}

// StartDiagnosticMonitor starts a goroutine that periodically logs diagnostic information
func StartDiagnosticMonitor(startTime time.Time, interval time.Duration, verbose bool) chan struct{} {
	stopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				if verbose {
					info := GetProcessInfo(startTime)
					log.Printf("DIAGNOSTIC: Goroutines: %d, Memory: %s/%s, GC cycles: %d",
						info.Goroutines, info.Memory.HeapInUse, info.Memory.HeapSys, info.Memory.NumGC)
				}
			}
		}
	}()

	return stopChan
}

// LogFullDiagnostics logs detailed diagnostic information
func LogFullDiagnostics(startTime time.Time) {
	info := GetProcessInfo(startTime)

	log.Printf("===== DIAGNOSTIC REPORT =====")
	log.Printf("PID: %d", info.PID)
	log.Printf("Go version: %s", info.GoVersion)
	log.Printf("CPU cores: %d", info.CPUCores)
	log.Printf("Goroutines: %d", info.Goroutines)
	log.Printf("Runtime: %s", info.ElapsedTime.Round(time.Second))
	log.Printf("Memory:")
	log.Printf("  - Alloc: %s", info.Memory.Alloc)
	log.Printf("  - TotalAlloc: %s", info.Memory.TotalAlloc)
	log.Printf("  - Sys: %s", info.Memory.Sys)
	log.Printf("  - HeapAlloc: %s", info.Memory.HeapAlloc)
	log.Printf("  - HeapSys: %s", info.Memory.HeapSys)
	log.Printf("  - HeapIdle: %s", info.Memory.HeapIdle)
	log.Printf("  - HeapInUse: %s", info.Memory.HeapInUse)
	log.Printf("  - StackInUse: %s", info.Memory.StackInUse)
	log.Printf("  - GC cycles: %d", info.Memory.NumGC)
	log.Printf("============================")
}

// formatBytes formats bytes as human-readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
