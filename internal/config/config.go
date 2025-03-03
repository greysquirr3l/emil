package config

// Config holds application configuration
type Config struct {
	SourceDir     string
	WorkerCount   int
	Verbose       bool
	RecursiveScan bool
	MaxMemoryPct  int // Added field for memory percentage limit

	// Attachment handling options
	SaveAttachments bool   // Whether to extract and save attachments
	AttachmentDir   string // Directory to save attachments in (if empty, use same dir as PDF)

	// Security options
	ScanAttachments bool   // Whether to scan attachments with ClamAV
	ClamdAddress    string // Address of ClamAV daemon (default: localhost:3310)
}
