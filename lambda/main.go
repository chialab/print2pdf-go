package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chialab/print2pdf-go/print2pdf"
)

// ResponseData represents a JSON-structured response.
type ResponseData struct {
	Url string `json:"url"`
}

// ResponseError represents a JSON-structured error response.
type ResponseError struct {
	Message string `json:"message"`
}

// Version string, set at build time.
var Version = "development"

// S3 bucket name. Required.
var BucketName = os.Getenv("BUCKET")

// Comma-separated list of allowed hosts for CORS requests. Defaults to "*", meaning all hosts.
var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

// Init function checks for required environment variables.
func init() {
	if len(os.Args) > 1 && slices.Contains([]string{"-v", "--version"}, os.Args[1]) {
		fmt.Printf("Version: %s\n", Version)
		os.Exit(0)
	}
	if BucketName == "" {
		fmt.Fprintln(os.Stderr, "missing required environment variable BUCKET")
		os.Exit(1)
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = print2pdf.StartBrowser(ctx); err != nil {
		return fmt.Errorf("error starting browser: %s", err)

	}

	lambda.StartWithOptions(handler, lambda.WithContext(ctx))

	<-ctx.Done()
	stop()

	return nil
}
