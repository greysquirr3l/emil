package config

// Config holds application configuration
type Config struct {
	SourceDir     string
	WorkerCount   int
	Verbose       bool
	RecursiveScan bool
	MaxMemoryPct  int // Added field for memory percentage limit
}
