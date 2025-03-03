package converter

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhillyerd/enmime"
	"github.com/jung-kurt/gofpdf"

	"emil/internal/config"
	"emil/internal/security"
)

// ConversionResult contains information about a converted file
type ConversionResult struct {
	InputPath      string
	OutputPath     string
	Success        bool
	Error          error
	Duration       time.Duration
	Attachments    []AttachmentResult
	SecurityAlerts []string
}

// ConvertEMLToPDF converts an EML file to PDF format with advanced options
func ConvertEMLToPDF(emlPath string, cfg *config.Config, scanner *security.Scanner) (*ConversionResult, error) {
	startTime := time.Now()
	result := &ConversionResult{
		InputPath: emlPath,
	}

	// Read and parse the EML file
	file, err := os.Open(emlPath)
	if err != nil {
		result.Error = fmt.Errorf("failed to open eml file: %w", err)
		return result, result.Error
	}
	defer file.Close()

	// Parse the email
	envelope, err := enmime.ReadEnvelope(file)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse eml content: %w", err)
		return result, result.Error
	}

	// Create PDF output file in the same directory
	pdfPath := strings.TrimSuffix(emlPath, filepath.Ext(emlPath)) + ".pdf"
	result.OutputPath = pdfPath

	// Determine attachment directory
	attachmentDir := cfg.AttachmentDir
	if attachmentDir == "" {
		// Default to same directory as PDF with _attachments suffix
		attachmentDir = strings.TrimSuffix(pdfPath, ".pdf") + "_attachments"
	}

	// Handle attachments if enabled
	if cfg.SaveAttachments && len(envelope.Attachments) > 0 {
		attachResults, err := HandleAttachments(envelope, attachmentDir, cfg.ScanAttachments, scanner)
		if err != nil {
			// Just log the error but continue with conversion
			if cfg.Verbose {
				fmt.Printf("Warning: %v\n", err)
			}
		}
		result.Attachments = attachResults

		// Check for security alerts
		for _, att := range attachResults {
			if att.ScanResult != nil && att.ScanResult.Infected {
				for _, threat := range att.ScanResult.Threats {
					alert := fmt.Sprintf("Security threat in %s: %s", att.Filename, threat)
					result.SecurityAlerts = append(result.SecurityAlerts, alert)
				}
			}
		}
	}

	// Check if we have HTML content to render with Chrome
	if envelope.HTML != "" {
		// Create a complete HTML document with headers, styles and email content
		htmlContent := buildCompleteHTML(envelope, result.Attachments)

		// Try to use chromedp for rich HTML rendering
		if err := renderHTMLToPDF(htmlContent, pdfPath); err == nil {
			result.Success = true
			result.Duration = time.Since(startTime)
			return result, nil // Successful HTML conversion
		} else if cfg.Verbose {
			fmt.Printf("Advanced HTML conversion failed, falling back to basic PDF: %v\n", err)
		}
	}

	// Fallback to basic PDF generation with gofpdf
	err = convertToBasicPDF(envelope, pdfPath, result.Attachments)
	if err != nil {
		result.Error = err
		return result, err
	}

	result.Success = true
	result.Duration = time.Since(startTime)
	return result, nil
}

// buildCompleteHTML creates a well-formed HTML document from email parts
func buildCompleteHTML(envelope *enmime.Envelope, attachments []AttachmentResult) string {
	var buffer bytes.Buffer

	// Start with HTML doctype and basic structure
	buffer.WriteString("<!DOCTYPE html>\n<html>\n<head>\n")
	buffer.WriteString("<meta charset=\"UTF-8\">\n")
	buffer.WriteString("<title>" + html.EscapeString(envelope.GetHeader("Subject")) + "</title>\n")

	// Add styles for email
	buffer.WriteString("<style>\n")
	buffer.WriteString("body { font-family: Arial, sans-serif; margin: 20px; }\n")
	buffer.WriteString(".email-header { margin-bottom: 20px; border-bottom: 1px solid #ccc; padding-bottom: 10px; }\n")
	buffer.WriteString(".header-row { margin: 5px 0; }\n")
	buffer.WriteString(".header-label { font-weight: bold; width: 60px; display: inline-block; }\n")
	buffer.WriteString(".email-body { margin-top: 20px; }\n")
	buffer.WriteString(".attachments { margin-top: 30px; border-top: 1px solid #eee; padding-top: 10px; }\n")
	buffer.WriteString(".attachment-item { margin: 5px 0; }\n")
	buffer.WriteString(".security-alert { color: red; font-weight: bold; }\n")
	buffer.WriteString("</style>\n")
	buffer.WriteString("</head>\n<body>\n")

	// Add email headers section
	buffer.WriteString("<div class=\"email-header\">\n")
	addHeader(&buffer, "From", envelope.GetHeader("From"))
	addHeader(&buffer, "To", envelope.GetHeader("To"))
	if cc := envelope.GetHeader("Cc"); cc != "" {
		addHeader(&buffer, "Cc", cc)
	}
	addHeader(&buffer, "Subject", envelope.GetHeader("Subject"))
	addHeader(&buffer, "Date", formatDate(envelope.GetHeader("Date")))
	buffer.WriteString("</div>\n")

	// Add email body
	buffer.WriteString("<div class=\"email-body\">\n")
	// Use original HTML content if available
	if envelope.HTML != "" {
		buffer.WriteString(envelope.HTML)
	} else if envelope.Text != "" {
		// Convert plain text to HTML paragraphs
		lines := strings.Split(envelope.Text, "\n")
		for _, line := range lines {
			if line == "" {
				buffer.WriteString("<br>\n")
			} else {
				buffer.WriteString(html.EscapeString(line) + "<br>\n")
			}
		}
	}
	buffer.WriteString("</div>\n")

	// Add attachments if any
	if len(attachments) > 0 {
		buffer.WriteString("<div class=\"attachments\">\n")
		buffer.WriteString("<h3>Attachments (" + fmt.Sprintf("%d", len(attachments)) + ")</h3>\n")
		buffer.WriteString("<ul>\n")
		for _, att := range attachments {
			buffer.WriteString("<li class=\"attachment-item\">")
			buffer.WriteString(html.EscapeString(att.Filename) + " (" + formatBytes(att.Size) + ")")

			// Add security alerts if present
			if att.ScanResult != nil && att.ScanResult.Infected {
				buffer.WriteString(" <span class=\"security-alert\">SECURITY THREAT DETECTED</span>")
			}

			buffer.WriteString("</li>\n")
		}
		buffer.WriteString("</ul>\n")
		buffer.WriteString("</div>\n")
	} else if len(envelope.Attachments) > 0 {
		// Fall back to envelope attachments if no processed attachments
		buffer.WriteString("<div class=\"attachments\">\n")
		buffer.WriteString("<h3>Attachments (" + fmt.Sprintf("%d", len(envelope.Attachments)) + ")</h3>\n")
		buffer.WriteString("<ul>\n")
		for _, att := range envelope.Attachments {
			buffer.WriteString("<li class=\"attachment-item\">" + html.EscapeString(att.FileName) +
				" (" + formatBytes(int64(len(att.Content))) + ")</li>\n")
		}
		buffer.WriteString("</ul>\n")
		buffer.WriteString("</div>\n")
	}

	buffer.WriteString("</body>\n</html>")
	return buffer.String()
}

// convertToBasicPDF creates a PDF using gofpdf
func convertToBasicPDF(envelope *enmime.Envelope, pdfPath string, attachments []AttachmentResult) error {
	// Create a new PDF document
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.AddPage()

	// Set up formatting
	pdf.SetFont("Arial", "B", 12)

	// Add email header information
	addEmailHeaders(pdf, envelope)

	// Add a divider line
	pdf.Line(10, pdf.GetY()+5, 200, pdf.GetY()+5)
	pdf.SetY(pdf.GetY() + 10)

	// Add email body (try HTML first, then plain text)
	if envelope.HTML != "" {
		addEnhancedHTMLContent(pdf, envelope.HTML)
	} else if envelope.Text != "" {
		addPlainTextContent(pdf, envelope.Text)
	}

	// Add attachment information with security alerts
	if len(attachments) > 0 {
		pdf.Ln(10)
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(0, 10, fmt.Sprintf("Attachments (%d):", len(attachments)))
		pdf.Ln(5)

		pdf.SetFont("Arial", "", 10)
		for _, att := range attachments {
			attackInfo := fmt.Sprintf("- %s (%s)", att.Filename, formatBytes(att.Size))
			pdf.Cell(0, 5, attackInfo)
			pdf.Ln(5)

			// Add security warnings for infected attachments
			if att.ScanResult != nil && att.ScanResult.Infected {
				pdf.SetTextColor(255, 0, 0) // Red text for warning
				pdf.Cell(0, 5, "  SECURITY ALERT: Malware detected in this attachment")
				pdf.SetTextColor(0, 0, 0) // Reset to black
				pdf.Ln(5)
			}
		}
	} else if len(envelope.Attachments) > 0 {
		// Fall back to basic attachment list
		addAttachmentsInfo(pdf, envelope.Attachments)
	}

	// Save the PDF
	err := pdf.OutputFileAndClose(pdfPath)
	if err != nil {
		return fmt.Errorf("failed to write pdf file: %w", err)
	}

	return nil
}

// addEmailHeaders adds email header information to the PDF
func addEmailHeaders(pdf *gofpdf.Fpdf, envelope *enmime.Envelope) {
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "From:")
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, envelope.GetHeader("From"))
	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "To:")
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, envelope.GetHeader("To"))
	pdf.Ln(10)

	if cc := envelope.GetHeader("Cc"); cc != "" {
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(40, 10, "Cc:")
		pdf.SetFont("Arial", "", 12)
		pdf.Cell(0, 10, cc)
		pdf.Ln(10)
	}

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "Subject:")
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, envelope.GetHeader("Subject"))
	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "Date:")
	pdf.SetFont("Arial", "", 12)

	// Try to parse and format the date
	if date := envelope.GetHeader("Date"); date != "" {
		if t, err := time.Parse(time.RFC1123Z, date); err == nil {
			date = t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
		}
		pdf.Cell(0, 10, date)
	}
	pdf.Ln(10)
}

// addEnhancedHTMLContent adds better HTML content to the PDF
func addEnhancedHTMLContent(pdf *gofpdf.Fpdf, htmlContent string) {
	pdf.SetFont("Arial", "", 11)

	// Extract text from HTML with improved formatting
	text := parseHTML(htmlContent)

	// Split into paragraphs and process each one
	paragraphs := strings.Split(text, "\n\n")
	for _, para := range paragraphs {
		if para == "" {
			continue
		}

		// Replace multiple spaces with single space
		para = strings.Join(strings.Fields(para), " ")

		// Add the paragraph text
		pdf.MultiCell(0, 5, para, "", "", false)
		pdf.Ln(3)
	}

	pdf.Ln(5)
}

// parseHTML does a more thorough job of converting HTML to formatted text
func parseHTML(html string) string {
	var result bytes.Buffer

	// Replace common block elements with newlines
	blockElements := []string{"</p>", "</div>", "</h1>", "</h2>", "</h3>", "</h4>", "</h5>", "</h6>", "</li>", "</tr>"}
	for _, tag := range blockElements {
		html = strings.ReplaceAll(html, tag, tag+"\n")
	}

	// Replace list items with bullets
	html = strings.ReplaceAll(html, "<li>", "• ")

	// Replace common HTML entities
	entities := map[string]string{
		"&nbsp;": " ",
		"&lt;":   "<",
		"&gt;":   ">",
		"&amp;":  "&",
		"&quot;": "\"",
		"&apos;": "'",
		"&#39;":  "'",
		"<br>":   "\n",
		"<br/>":  "\n",
		"<br />": "\n",
	}
	for entity, replacement := range entities {
		html = strings.ReplaceAll(html, entity, replacement)
	}

	// Remove all HTML tags
	inTag := false
	for _, c := range html {
		if c == '<' {
			inTag = true
		} else if c == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(c)
		}
	}

	// Normalize whitespace
	lines := strings.Split(result.String(), "\n")
	var normalized []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}

	// Group content into paragraphs
	content := strings.Join(normalized, "\n")
	content = strings.ReplaceAll(content, "\n\n\n", "\n\n")

	return content
}

// addHeader adds an email header line to the HTML buffer
func addHeader(buffer *bytes.Buffer, label, value string) {
	buffer.WriteString(fmt.Sprintf("<div class=\"header-row\"><span class=\"header-label\">%s</span> %s</div>\n",
		label, html.EscapeString(value)))
}

// addPlainTextContent adds plain text email body to the PDF
func addPlainTextContent(pdf *gofpdf.Fpdf, textContent string) {
	pdf.SetFont("Arial", "", 11)
	pdf.MultiCell(0, 5, textContent, "", "", false)
	pdf.Ln(5)
}

// addAttachmentsInfo adds information about attachments to the PDF
func addAttachmentsInfo(pdf *gofpdf.Fpdf, attachments []*enmime.Part) {
	pdf.Ln(10)
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(0, 10, fmt.Sprintf("Attachments (%d):", len(attachments)))
	pdf.Ln(5)

	pdf.SetFont("Arial", "", 10)
	for _, att := range attachments {
		pdf.Cell(0, 5, fmt.Sprintf("- %s (%s)", att.FileName, formatBytes(int64(len(att.Content)))))
		pdf.Ln(5)
	}
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

// formatDate parses and formats an email date header
func formatDate(date string) string {
	if t, err := time.Parse(time.RFC1123Z, date); err == nil {
		return t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
	}
	return date // return original if parsing fails
}
