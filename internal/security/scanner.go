package security

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	clamd "github.com/dutchcoders/go-clamd"
)

// Scanner provides virus scanning capabilities
type Scanner struct {
	enabled bool
	client  *clamd.Clamd
}

// ScanResult contains the result of a virus scan
type ScanResult struct {
	Scanned  bool
	Infected bool
	Threats  []string
}

// NewScanner creates a new virus scanner
func NewScanner(enabled bool, clamdAddress string) (*Scanner, error) {
	// Use default address if empty
	if clamdAddress == "" {
		clamdAddress = "localhost:3310"
	}

	// Check if ClamAV is installed and running
	if !isClamAVAvailable() {
		if enabled {
			fmt.Println("ClamAV is not available, disabling virus scanning.")
		}
		return &Scanner{enabled: false}, nil
	}

	// Create ClamAV client
	client := clamd.NewClamd(clamdAddress)

	// Test connection
	version, err := client.Version()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClamAV: %w", err)
	}

	// Successfully connected
	if len(version) > 0 {
		return &Scanner{
			enabled: true,
			client:  client,
		}, nil
	}

	return nil, fmt.Errorf("unknown error initializing ClamAV scanner")
}

// isClamAVAvailable checks if ClamAV is installed and the daemon is running
func isClamAVAvailable() bool {
	// Check if clamscan is in the PATH
	cmd := exec.Command("clamscan", "--version")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Check if we can connect to clamd
	client := clamd.NewClamd("localhost:3310")
	if err := client.Ping(); err != nil {
		return false
	}

	return true
}

// IsEnabled returns whether the scanner is enabled
func (s *Scanner) IsEnabled() bool {
	return s.enabled
}

// ScanFile scans a file for viruses
func (s *Scanner) ScanFile(filePath string) (*ScanResult, error) {
	if !s.enabled {
		return &ScanResult{Scanned: false}, nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for scanning: %w", err)
	}
	defer file.Close()

	return s.ScanReader(file)
}

// ScanBytes scans a byte slice for viruses
func (s *Scanner) ScanBytes(data []byte) (*ScanResult, error) {
	if !s.enabled {
		return &ScanResult{Scanned: false}, nil
	}

	return s.ScanReader(bytes.NewReader(data))
}

// ScanReader scans an io.Reader for viruses
func (s *Scanner) ScanReader(reader io.Reader) (*ScanResult, error) {
	if !s.enabled {
		return &ScanResult{Scanned: false}, nil
	}

	result := &ScanResult{
		Scanned: true,
		Threats: []string{},
	}

	// Scan the reader
	scanResults, err := s.client.ScanStream(reader, make(chan bool))
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}

	// Process scan results
	for sr := range scanResults {
		if sr.Status == "FOUND" {
			result.Infected = true
			result.Threats = append(result.Threats, fmt.Sprintf("%s: %s", sr.Description, sr.Status))
		}
	}

	return result, nil
}
