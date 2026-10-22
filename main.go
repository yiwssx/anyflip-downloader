package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

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

	if err := validateDownloadSettings(donwloadThreads, downloadRetries, downloadRetryDelay, chunkSize); err != nil {
		log.Fatal(err)
	}

	anyflipURL, err := url.Parse(flag.Args()[0])
	if err != nil {
		log.Fatal(err)
	}

	if insecure {
		fmt.Println("You enabled insecure downloads. This disables security checks. Stay safe!")
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	fmt.Println("Preparing to download")
	flipbook, err := prepareDownload(anyflipURL)
	if err != nil {
		log.Fatal(err)
	}

	outputFile := flipbook.title + ".pdf"
	if _, err := os.Stat(outputFile); err == nil {
		log.Fatalf("output file %q already exists; refusing to overwrite it", outputFile)
	} else if !os.IsNotExist(err) {
		log.Fatalf("cannot inspect output file: %v", err)
	}

	// Every run owns a newly created workspace. Never reuse an existing folder:
	// deleting a caller-owned directory would destroy unrelated data.
	workDir, cleanupWorkspace, err := createDownloadWorkspace(tempDownloadFolder)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Temporary workspace: %s\\n", workDir)

	if err := flipbook.downloadImages(
		workDir,
		downloadOptions{threads: donwloadThreads, retries: downloadRetries, retryDelay: downloadRetryDelay},
	); err != nil {
		log.Fatalf("download failed (images kept at %s): %v", workDir, err)
	}
	fmt.Println("Converting to pdf")
	if err := createPDF(outputFile, workDir, int(chunkSize)); err != nil {
		log.Fatalf("PDF conversion failed (images kept at %s): %v", workDir, err)
	}

	if !keepDownloadFolder {
		if err := cleanupWorkspace(); err != nil {
			log.Printf("warning: could not remove temporary workspace %q: %v", workDir, err)
		}
	}

	fmt.Println("Done")
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
