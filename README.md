# Emil: EML to PDF Converter

Emil is a high-performance, multi-threaded tool for converting EML email files to richly formatted PDFs. It features dynamic worker scaling, self-healing capabilities, and comprehensive progress reporting.

## Features

- **Fast conversion**: Utilizes multiple worker threads to process files in parallel
- **Resource-aware**: Dynamically scales worker count based on system resource usage
- **Self-healing**: Workers automatically recover from failures
- **Detailed reporting**: Real-time progress updates and comprehensive statistics
- **Preserves formatting**: Maintains email structure and formatting in generated PDFs

## Installation

### Option 1: Using Go Install

```bash
go install github.com/greysqurr3l/emil/cmd/emil@latest
```

### Option 2: Clone and Build Manually

```bash
git clone https://github.com/greysqurr3l/emil.git
cd emil
go mod download  # Download all dependencies
go build ./cmd/emil
```

## Dependencies

Emil relies on several open-source libraries:

- github.com/jhillyerd/enmime - For parsing EML files
- github.com/jung-kurt/gofpdf - For creating PDF documents
- github.com/schollz/progressbar/v3 - For displaying progress information

## Usage

```
./emil [options]
```

### Options

```
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
```

### Examples

Convert all EML files in the current directory:

```
./emil
```

Convert EML files in a specific directory with verbose output:

```
./emil -src /path/to/emails -verbose
```

Use 8 worker threads and diagnostic monitoring:

```
./emil -workers 8 -diagnose
```

Limit memory usage to 50% and enable verbose logging:

```
./emil -max-mem 50 -verbose
```

## Performance Tuning

Emil automatically scales the number of workers based on system resources, but you can tune its behavior:

- Start with `-workers` set to your CPU core count for optimal performance
- Use `-max-mem` to adjust memory usage threshold for worker scaling
- Enable `-diagnose` to monitor resource usage during processing

## How It Works

1. Emil scans the input directory for EML files
2. Files are queued for processing
3. Worker pool converts files using parallel processing
4. Resource manager dynamically adjusts worker count
5. Real-time statistics track progress and performance
6. Self-healing mechanisms recover from failures

## License

MIT
