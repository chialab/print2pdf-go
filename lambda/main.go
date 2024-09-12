package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/aws/aws-lambda-go/events"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := print2pdf.StartBrowser(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error starting browser: %s\n", err)
		os.Exit(1)
	}

	lambda.Start(handler)

	<-ctx.Done()
	stop()
}

// Handle a request.
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	if CorsAllowedHosts == "" || CorsAllowedHosts == "*" {
		headers["Access-Control-Allow-Origin"] = "*"
	} else {
		allowedHosts := strings.Split(CorsAllowedHosts, ",")
		origin := event.Headers["Origin"]
		if origin == "" {
			origin = event.Headers["origin"]
		}
		if slices.Contains(allowedHosts, origin) {
			headers["Access-Control-Allow-Origin"] = origin
		}
	}

	var data print2pdf.GetPDFParams
	err := json.Unmarshal([]byte(event.Body), &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error decoding JSON: %s\noriginal request body: %s\n", err, event.Body)

		return JsonError("internal server error", 500), nil
	}
	if !strings.HasSuffix(data.FileName, ".pdf") {
		data.FileName += ".pdf"
	}

	h, err := print2pdf.NewS3Handler(ctx, BucketName, data.FileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating print handler: %s\n", err)

		return JsonError("internal server error", 500), nil
	}

	url, err := print2pdf.PrintPDF(ctx, data, h)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)

		return JsonError(ve.Error(), 400), nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF: %s\n", err)

		return JsonError("internal server error", 500), nil
	}

	body, err := json.Marshal(ResponseData{Url: url})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding response to JSON: %s\n", err)

		return JsonError("internal server error", 500), nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(body),
		Headers:    headers,
	}, nil
}

// Prepare an HTTP error response.
func JsonError(message string, code int) events.APIGatewayProxyResponse {
	ct := "application/json"
	body, err := json.Marshal(ResponseError{message})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding error message to JSON: %s\noriginal error: %s\n", err, message)
		body = []byte("internal server error")
		code = 500
		ct = "text/plain"
	}

	return events.APIGatewayProxyResponse{
		StatusCode: code,
		Body:       string(body),
		Headers: map[string]string{
			"Content-Type":           ct,
			"X-Content-Type-Options": "nosniff",
		},
	}
}
