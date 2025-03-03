# Emil: EML to PDF Converter

Emil is a high-performance, multi-threaded tool for converting EML email files to richly formatted PDFs. It features dynamic worker scaling, self-healing capabilities, comprehensive progress reporting, attachment handling, and security scanning.

## Features

- Fast conversion: Utilizes multiple worker threads to process files in parallel
- Resource-aware: Dynamically scales worker count based on system resource usage
- Self-healing: Workers automatically recover from failures
- Detailed reporting: Real-time progress updates and comprehensive statistics
- Rich HTML rendering: Properly renders HTML emails with full CSS support
- Attachment handling: Extracts and saves email attachments
- Security scanning: Optional virus scanning for email attachments (ClamAV)
- Fallback rendering: Works even without Chrome installed

## Installation

### Prerequisites

Emil requires the following non-Go resources:

1. **Chrome or Chromium**: For optimal HTML rendering, Chrome or Chromium should be installed.

    - **macOS:**

        ```bash
        brew install --cask google-chrome
        # or
        brew install chromium
        ```

    - **Windows:**

        - Download Chrome from: <https://www.google.com/chrome/>
        - Download Chromium from: <https://www.chromium.org/getting-involved/download-chromium/> (navigate to the "Latest Build" link for your architecture)

        After installing, ensure that the Chrome or Chromium executable is in your system's `PATH` environment variable.

    - **Linux (Ubuntu/Debian):**

        ```bash
        sudo apt update
        sudo apt install chromium-browser
        ```

2. **ClamAV (Optional)**: For virus scanning of attachments, ClamAV should be installed and running.

    - **macOS:**

    ```bash
    brew install clamav
    brew services start clamav
    freshclam
    ```

    To ensure ClamAV starts on boot, run:

    ```bash
    brew services run clamav
    ```

    **Windows:**

    1. Download the latest ClamAV installer from a trusted source (e.g., SourceForge).
    2. Run the installer and follow the prompts.
    3. Update the virus definitions by running `freshclam.exe` from the ClamAV installation directory.
    4. Start the ClamAV service manually or configure it to start automatically.

    After installing, ensure that the ClamAV executables (e.g., `clamscan.exe`) are in your system's `PATH` environment variable.

    **Linux (Ubuntu/Debian):**

    ```bash
    sudo apt update
    sudo apt install clamav clamav-daemon
    sudo systemctl start clamav-daemon
    sudo freshclam
    ```

    To ensure ClamAV starts on boot, run:

    ```bash
    sudo systemctl enable clamav-daemon
    ```

### Option 1: Using Go Install

```bash
go install github.com/greysqurr3l/emil/cmd/emil@latest
```

### Option 2: Clone and Build Manually

```bash
git clone https://github.com/greysqurr3l/emil.git
cd emil
go mod download
go build -o emil cmd/emil/main.go
```

## Dependencies

Emil relies on several open-source libraries:

- [jhillyerd/enmime](https://github.com/jhillyerd/enmime) - For parsing EML files
- [chromedp/chromedp](https://github.com/chromedp/chromedp) - For high-quality HTML to PDF conversion
- [jung-kurt/gofpdf](https://github.com/jung-kurt/gofpdf) - For creating fallback PDF documents
- [schollz/progressbar/v3](https://github.com/schollz/progressbar/v3) - For displaying progress information
- [dutchcoders/go-clamd](https://github.com/dutchcoders/go-clamd) - For ClamAV integration (virus scanning)

## Usage

```bash
./emil [options]
```

### Options

```bash
-src string
    Source directory to scan for EML files (default ".")
-workers int
    Initial number of worker threads (default: number of CPU cores)
-recursive
    Recursively scan directories (default true)
-verbose
    Enable verbose output (default false)
-diagnose
    Show diagnostic information (default false)
-max-mem int
    Maximum memory usage percentage target (default 75)
-test
    Test mode - convert only the first EML file found and exit

# Attachment Options
-attachments
    Save email attachments (default true)
-attachment-dir string
    Directory for saving attachments (default: alongside PDFs)

# Security Options
-scan
    Scan attachments for viruses using ClamAV (default false, enabled if available)
-clamd string
    ClamAV daemon address (default "localhost:3310")
```

### Examples

Convert all EML files in the current directory:

```bash
./emil
```

Convert with security scanning enabled:

```bash
./emil -scan
```

Specify attachment directory and enable verbose output:

```bash
./emil -attachment-dir ./attachments -verbose
```

Use 8 worker threads and diagnostic monitoring:

```bash
./emil -workers 8 -diagnose
```

Run in test mode with attachment saving and scanning:

```bash
./emil -test -attachments -scan -src /path/to/emails
```

## Performance Tuning

Emil automatically scales the number of workers based on system resources, but you can tune its behavior:

- Start with `-workers` set to your CPU core count for optimal performance
- Use `-max-mem` to adjust memory usage threshold for worker scaling
- Enable `-diagnose` to monitor resource usage during processing

## Troubleshooting

If you encounter issues, try the following:

- Run in test mode (`-test`) to verify basic conversion works
- Use verbose mode (`-verbose`) to see detailed logs
- Enable diagnostics (`-diagnose`) to monitor resource usage
- Reduce the number of workers if memory usage is too high
- Ensure Chrome or Chromium is properly installed if HTML rendering fails
- Ensure ClamAV is properly installed and running if using `-scan`

## License

MIT
