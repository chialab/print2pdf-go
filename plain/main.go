package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/chialab/print2pdf-go/print2pdf"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type ResponseData struct {
	Url string `json:"url"`
}

type ResponseError struct {
	Message string `json:"message"`
}

// Version string, set at build time.
var Version = "development"

// S3 bucket name. Required for "/v1/print" endpoint.
var BucketName = os.Getenv("BUCKET")

// Webserver port. Defaults to 3000.
var Port = os.Getenv("PORT")

// Comma-separated list of allowed hosts for CORS requests. Defaults to "*", meaning all hosts.
var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

// Comma-separated list of cookies to forward when navigating to the URL to be printed.
var ForwardCookies = os.Getenv("FORWARD_COOKIES")

// Comma-separated list of hosts for which printing is allowed.
var PrintAllowedHosts = os.Getenv("PRINT_ALLOWED_HOSTS")

// Function to shutdown OpenTelemetry.
var otelShutdown func(context.Context) error

// Init function set default values to environment variables and initialized OpenTelemetry SDK.
func init() {
	if len(os.Args) > 1 && slices.Contains([]string{"-v", "--version"}, os.Args[1]) {
		fmt.Printf("Version: %s\n", Version)
		os.Exit(0)
	}
	if Port == "" {
		Port = "3000"
	}

	var err error
	otelShutdown, err = setupOTelSDK()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing OpenTelemetry: %s\n", err)
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
	defer func() {
		err = errors.Join(err, otelShutdown(context.Background()))
	}()
	if err = print2pdf.StartBrowser(ctx); err != nil {
		return fmt.Errorf("error starting browser: %s", err)
	}

	srv := &http.Server{
		Addr:        ":" + Port,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
		ReadTimeout: 10 * time.Second,
		Handler:     newHTTPHandler(),
	}
	srvErr := make(chan error, 1)
	go func() {
		fmt.Printf("server listening on port %s\n", Port)
		srvErr <- srv.ListenAndServe()
	}()

	// Wait for a server error or interrupt signal.
	select {
	case err = <-srvErr:
		return fmt.Errorf("error starting server: %s", err)
	case <-ctx.Done():
		stop()
	}

	if err = srv.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("error closing server: %s", err)
	}

	return nil
}

// Create an HTTP handler instrumented by OpenTelemetry.
func newHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/status", http.HandlerFunc(statusHandler))
	mux.Handle("/v1/print", http.HandlerFunc(printV1Handler))
	mux.Handle("/v2/print", http.HandlerFunc(printV2Handler))
	mux.Handle("/metrics", promhttp.Handler())

	return otelhttp.NewHandler(mux, "/")
}

// Setup OpenTelemetry SDK.
func setupOTelSDK() (func(context.Context) error, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("print2pdf"),
			semconv.ServiceVersion(Version),
		),
	)
	if err != nil {
		return nil, err
	}

	metricExporter, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metricExporter),
	)
	otel.SetMeterProvider(meterProvider)

	return meterProvider.Shutdown, nil
}
