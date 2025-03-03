package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jhillyerd/enmime"

	"emil/internal/security"
)

// AttachmentResult contains information about a processed attachment
type AttachmentResult struct {
	Filename    string
	Size        int64
	ContentType string
	SavedPath   string
	ScanResult  *security.ScanResult
}

// HandleAttachments extracts and optionally scans email attachments
func HandleAttachments(envelope *enmime.Envelope, outputDir string, scan bool, scanner *security.Scanner) ([]AttachmentResult, error) {
	results := []AttachmentResult{}

	// If no attachments, return empty result
	if len(envelope.Attachments) == 0 {
		return results, nil
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create attachment directory: %w", err)
	}

	// Process each attachment
	for _, att := range envelope.Attachments {
		// Create basic result
		result := AttachmentResult{
			Filename:    sanitizeFilename(att.FileName),
			Size:        int64(len(att.Content)),
			ContentType: att.ContentType,
		}

		// Determine safe output path
		result.SavedPath = filepath.Join(outputDir, result.Filename)

		// Ensure unique filename
		result.SavedPath = ensureUniqueFilename(result.SavedPath)

		// Save the attachment
		if err := os.WriteFile(result.SavedPath, att.Content, 0644); err != nil {
			return results, fmt.Errorf("failed to save attachment %s: %w", att.FileName, err)
		}

		// Scan for viruses if requested
		if scan && scanner != nil && scanner.IsEnabled() {
			scanResult, err := scanner.ScanFile(result.SavedPath)
			if err != nil {
				return results, fmt.Errorf("failed to scan attachment %s: %w", att.FileName, err)
			}
			result.ScanResult = scanResult

			// If infected, optionally rename or quarantine
			if scanResult.Infected {
				// Add .infected extension
				infectedPath := result.SavedPath + ".infected"
				if err := os.Rename(result.SavedPath, infectedPath); err != nil {
					return results, fmt.Errorf("failed to mark infected file %s: %w", att.FileName, err)
				}
				result.SavedPath = infectedPath
			}
		}

		// Add to results
		results = append(results, result)
	}

	return results, nil
}

// sanitizeFilename makes a filename safe for use on the filesystem
func sanitizeFilename(filename string) string {
	// Replace invalid characters with underscore
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := filename

	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}

	return result
}

// ensureUniqueFilename ensures the filename doesn't conflict with existing files
func ensureUniqueFilename(path string) string {
	// If file doesn't exist, return original
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	// File exists, add number to make unique
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	name := strings.TrimSuffix(filepath.Base(path), ext)

	counter := 1
	for {
		newPath := filepath.Join(dir, fmt.Sprintf("%s_%d%s", name, counter, ext))
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
		counter++
	}
}
