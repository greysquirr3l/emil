package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// renderHTMLToPDF uses headless Chrome to convert HTML to PDF with proper rendering
func renderHTMLToPDF(htmlContent string, outputPath string) error {
	// Create a temporary HTML file to render
	tmpDir, err := os.MkdirTemp("", "emil-html")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpHTML := filepath.Join(tmpDir, "email.html")
	if err := os.WriteFile(tmpHTML, []byte(htmlContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp HTML file: %w", err)
	}

	// Convert file path to URL format
	fileURL := fmt.Sprintf("file://%s", tmpHTML)

	// Create context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create browser instance
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.DisableGPU,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-web-security", true),
		chromedp.Flag("no-sandbox", true),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()

	taskCtx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Ensure that the browser is started
	if err := chromedp.Run(taskCtx); err != nil {
		return fmt.Errorf("failed to start browser: %w", err)
	}

	// Generate PDF from HTML
	var pdfBuffer []byte
	if err := chromedp.Run(taskCtx,
		chromedp.Navigate(fileURL),
		chromedp.WaitReady("body"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Wait a bit for any additional resources to load
			time.Sleep(500 * time.Millisecond)
			return nil
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Generate PDF data
			resp, _, err := page.PrintToPDF().WithPrintBackground(true).Do(ctx)
			if err != nil {
				return err
			}
			pdfBuffer = resp
			return nil
		}),
	); err != nil {
		return fmt.Errorf("failed to generate PDF: %w", err)
	}

	// Write the PDF file
	if err := os.WriteFile(outputPath, pdfBuffer, 0644); err != nil {
		return fmt.Errorf("failed to write PDF file: %w", err)
	}

	return nil
}
