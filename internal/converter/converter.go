package converter

import (
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhillyerd/enmime"
	"github.com/jung-kurt/gofpdf"
)

// ConvertEMLToPDF converts an EML file to PDF format
func ConvertEMLToPDF(emlPath string) error {
	// Read and parse the EML file
	file, err := os.Open(emlPath)
	if err != nil {
		return fmt.Errorf("failed to open eml file: %w", err)
	}
	defer file.Close()

	// Parse the email
	envelope, err := enmime.ReadEnvelope(file)
	if err != nil {
		return fmt.Errorf("failed to parse eml content: %w", err)
	}

	// Parse email headers
	msg, err := mail.ReadMessage(strings.NewReader(envelope.Raw))
	if err != nil {
		return fmt.Errorf("failed to parse email headers: %w", err)
	}

	// Create PDF output file in the same directory
	pdfPath := strings.TrimSuffix(emlPath, filepath.Ext(emlPath)) + ".pdf"

	// Create a new PDF document
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(10, 10, 10)
	pdf.AddPage()

	// Set up formatting
	pdf.SetFont("Arial", "B", 12)

	// Add email header information
	addEmailHeaders(pdf, msg)

	// Add a divider line
	pdf.Line(10, pdf.GetY()+5, 200, pdf.GetY()+5)
	pdf.SetY(pdf.GetY() + 10)

	// Add email body (try HTML first, then plain text)
	if envelope.HTML != "" {
		addHTMLContent(pdf, envelope.HTML)
	} else if envelope.Text != "" {
		addPlainTextContent(pdf, envelope.Text)
	}

	// Add attachments information
	if len(envelope.Attachments) > 0 {
		addAttachmentsInfo(pdf, envelope.Attachments)
	}

	// Save the PDF
	err = pdf.OutputFileAndClose(pdfPath)
	if err != nil {
		return fmt.Errorf("failed to write pdf file: %w", err)
	}

	return nil
}

// addEmailHeaders adds email header information to the PDF
func addEmailHeaders(pdf *gofpdf.Fpdf, msg *mail.Message) {
	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "From:")
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, msg.Header.Get("From"))
	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "To:")
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, msg.Header.Get("To"))
	pdf.Ln(10)

	if cc := msg.Header.Get("Cc"); cc != "" {
		pdf.SetFont("Arial", "B", 12)
		pdf.Cell(40, 10, "Cc:")
		pdf.SetFont("Arial", "", 12)
		pdf.Cell(0, 10, cc)
		pdf.Ln(10)
	}

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "Subject:")
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(0, 10, msg.Header.Get("Subject"))
	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 12)
	pdf.Cell(40, 10, "Date:")
	pdf.SetFont("Arial", "", 12)

	// Try to parse and format the date
	if date := msg.Header.Get("Date"); date != "" {
		if t, err := time.Parse(time.RFC1123Z, date); err == nil {
			date = t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
		}
		pdf.Cell(0, 10, date)
	}
	pdf.Ln(10)
}

// addHTMLContent adds HTML email body content to the PDF
// This is a simplified version that extracts text from HTML
func addHTMLContent(pdf *gofpdf.Fpdf, htmlContent string) {
	pdf.SetFont("Arial", "", 11)

	// For a real implementation, you'd want to use a HTML-to-PDF library
	// This is a simplified version that just adds the text
	plainText := extractTextFromHTML(htmlContent)

	// Write multiline text
	pdf.MultiCell(0, 5, plainText, "", "", false)
	pdf.Ln(5)
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

// extractTextFromHTML does a basic conversion of HTML to plain text
// In a real application, use a proper HTML parser
func extractTextFromHTML(htmlContent string) string {
	// This is an extremely simplified version
	// In reality, you'd want to use a proper HTML parser
	text := htmlContent

	// Remove HTML tags (very basic implementation)
	for {
		startIdx := strings.Index(text, "<")
		if startIdx == -1 {
			break
		}
		endIdx := strings.Index(text, ">")
		if endIdx == -1 {
			break
		}
		text = text[:startIdx] + text[endIdx+1:]
	}

	// Replace common HTML entities
	replacements := map[string]string{
		"&nbsp;": " ",
		"&lt;":   "<",
		"&gt;":   ">",
		"&amp;":  "&",
		"&quot;": "\"",
		"&apos;": "'",
		"<br>":   "\n",
		"<br/>":  "\n",
		"<br />": "\n",
	}

	for entity, replacement := range replacements {
		text = strings.ReplaceAll(text, entity, replacement)
	}

	// Normalize whitespace
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}

	return strings.Join(lines, "\n")
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
