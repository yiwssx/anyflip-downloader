package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/schollz/progressbar/v3"
)

var title string
var tempDownloadFolder string
var insecure bool
var keepDownloadFolder bool
var donwloadThreads int
var downloadRetries int
var downloadRetryDelay time.Duration
var chunkSize uint

type flipbook struct {
	URL       *url.URL
	title     string
	pageCount int
	pageURLs  []string
}

type downloadOptions struct {
	threads    int
	retries    int
	retryDelay time.Duration
}

func init() {
	flag.Usage = printUsage
	flag.StringVar(&tempDownloadFolder, "temp-download-folder", "", "Specifies the name of the temporary download folder")
	flag.StringVar(&title, "title", "", "Specifies the name of the generated PDF document (uses book title if not specified)")
	flag.BoolVar(&insecure, "insecure", false, "Skip certificate validation")
	flag.BoolVar(&keepDownloadFolder, "keep-download-folder", false, "Keep the temporary download folder instead of deleting it after completion")
	flag.IntVar(&donwloadThreads, "threads", 1, "Number of parallel download processes")
	flag.IntVar(&downloadRetries, "retries", 1, "Number of download retries")
	flag.DurationVar(&downloadRetryDelay, "waitretry", time.Second, "Wait time between download retries")
	flag.UintVar(&chunkSize, "chunksize", 10, "Amount of images converted at once. Higher amount amount will end in less write actions but more memory usage")
}

func main() {
	flag.Parse()
	run()
}

func run() {
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "error: no URL provided")
		printUsage()
		os.Exit(1)
	}

	if insecure {
		fmt.Println("Warning: insecure CLI mode disables TLS verification; do not use it on a hosted server.")
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("Preparing to download")
	output, err := RunDownloadJob(ctx, DownloadJobOptions{
		URL:                flag.Args()[0],
		Title:              title,
		TempDownloadFolder: tempDownloadFolder,
		KeepImages:         keepDownloadFolder,
		Threads:            donwloadThreads,
		Retries:            downloadRetries,
		RetryDelay:         downloadRetryDelay,
		ChunkSize:          chunkSize,
	}, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Done: %s\n", output)
}

func printUsage() {
	w := flag.CommandLine.Output()
	fmt.Fprintf(w, "Usage:\n")
	fmt.Fprintf(w, "  %s [OPTIONS] <url>\n", os.Args[0])
	fmt.Fprintf(w, "Options:\n")
	flag.PrintDefaults()
}

func createPDF(outputFile string, imageDir string, chunkSize int) error {
	if chunkSize < 1 {
		return fmt.Errorf("chunk size must be at least 1")
	}
	if _, err := os.Stat(outputFile); err == nil {
		return fmt.Errorf("output file %q already exists; refusing to overwrite it", outputFile)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot inspect output file %q: %w", outputFile, err)
	}

	files, err := os.ReadDir(imageDir)
	if err != nil {
		return err
	}

	var imagePaths []string

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := filepath.Ext(file.Name())
		if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
			imagePaths = append(imagePaths, filepath.Join(imageDir, file.Name()))
		}
	}

	if len(imagePaths) == 0 {
		return fmt.Errorf("no images found in path %s", imageDir)
	}

	bar := progressbar.NewOptions(len(imagePaths),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionShowCount(),
		progressbar.OptionSetDescription("Converting"),
	)
	defer bar.Close()

	impConf := pdfcpu.DefaultImportConfig()

	for i := 0; i < len(imagePaths); i += chunkSize {
		end := min(i+chunkSize, len(imagePaths))

		imagePathChunk := imagePaths[i:end]
		if err := api.ImportImagesFile(imagePathChunk, outputFile, impConf, nil); err != nil {
			return fmt.Errorf("cannot import image pages %d-%d: %w", i+1, end, err)
		}
		bar.Add(min(chunkSize, len(imagePaths)-i))
	}

	fmt.Println()
	return nil
}
