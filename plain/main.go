package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/chialab/print2pdf-go/print2pdf"
)

type ResponseData struct {
	Url string `json:"url"`
}

type ResponseError struct {
	Message string `json:"message"`
}

// S3 bucket name. Required for "/v1/print" endpoint.
var BucketName = os.Getenv("BUCKET")

// Webserver port. Defaults to 3000.
var Port = os.Getenv("PORT")

// Comma-separated list of allowed hosts for CORS requests. Defaults to "*", meaning all hosts.
var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

// Init function set default values to environment variables.
func init() {
	if Port == "" {
		Port = "3000"
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := print2pdf.StartBrowser(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error starting browser: %s\n", err)
		os.Exit(1)
	}

	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/v1/print", printV1Handler)
	http.HandleFunc("/v2/print", printV2Handler)

	srv := &http.Server{
		Addr:        ":" + Port,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
		ReadTimeout: 10 * time.Second,
		Handler:     http.DefaultServeMux,
	}
	srvErr := make(chan error, 1)
	go func() {
		fmt.Printf("server listening on port %s\n", Port)
		srvErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-srvErr:
		fmt.Fprintf(os.Stderr, "error starting server: %s\n", err)
		os.Exit(1)
	case <-ctx.Done():
		stop()
	}

	err := srv.Shutdown(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "error closing server: %s\n", err)
		os.Exit(1)
	}
}

// Handle requests to "/status" endpoint.
func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)

		return
	}

	if !print2pdf.Running() {
		w.WriteHeader(http.StatusServiceUnavailable)

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Handle requests to "/v1/print" endpoint.
func printV1Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		handlePrintOptions(w, r)

	case "POST":
		if BucketName == "" {
			fmt.Fprintln(os.Stderr, "missing required environment variable BUCKET")
			JsonError(w, "internal server error", http.StatusInternalServerError)

			return
		}

		handlePrintV1Post(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Handle requests to "/v2/print" endpoint.
func printV2Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		handlePrintOptions(w, r)

	case "POST":
		handlePrintV2Post(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Handle OPTIONS requests.
func handlePrintOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "OPTIONS,POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	writeCorsOriginHeader(w, r.Header.Get("Origin"))
	w.WriteHeader(http.StatusOK)
}

// Handle POST requests to "/v1/print" endpoint.
func handlePrintV1Post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	writeCorsOriginHeader(w, r.Header.Get("Origin"))
	data, err := readRequest(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	h, err := print2pdf.NewS3Handler(r.Context(), BucketName, data.FileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating print handler: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	res, err := print2pdf.PrintPDF(r.Context(), data, h)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)
		JsonError(w, ve.Error(), http.StatusBadRequest)

		return
	} else if errors.Is(r.Context().Err(), context.Canceled) {
		fmt.Println("connection closed or request canceled")

		return
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	body, err := json.Marshal(ResponseData{Url: res})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding response to JSON: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	_, err = w.Write(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing response: %s\n", err)
	}
}

// Handle POST requests to "/v2/print" endpoint.
func handlePrintV2Post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/pdf")
	writeCorsOriginHeader(w, r.Header.Get("Origin"))
	data, err := readRequest(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", data.FileName))
	h := print2pdf.NewStreamHandler(w)
	_, err = print2pdf.PrintPDF(r.Context(), data, h)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)
		JsonError(w, ve.Error(), http.StatusBadRequest)
	} else if errors.Is(r.Context().Err(), context.Canceled) {
		fmt.Println("connection closed or request canceled")
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)
	}
}

// Read request parameters in structure.
func readRequest(r *http.Request) (print2pdf.GetPDFParams, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return print2pdf.GetPDFParams{}, fmt.Errorf("error reading request data: %s", err)
	}

	var data print2pdf.GetPDFParams
	err = json.Unmarshal(body, &data)
	if err != nil {
		return print2pdf.GetPDFParams{}, fmt.Errorf("error decoding JSON: %s\noriginal request body: %s", err, string(body))
	}

	if !strings.HasSuffix(data.FileName, ".pdf") {
		data.FileName += ".pdf"
	}

	return data, nil
}

// Write the "Access-Control-Allow-Origin" header.
func writeCorsOriginHeader(w http.ResponseWriter, origin string) {
	if CorsAllowedHosts == "" || CorsAllowedHosts == "*" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		allowedHosts := strings.Split(CorsAllowedHosts, ",")
		if slices.Contains(allowedHosts, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
	}
}

// JsonError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func JsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	body, err := json.Marshal(ResponseError{message})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding error message to JSON: %s\noriginal error: %s\n", err, message)
		body = []byte("internal server error")
		code = http.StatusInternalServerError
		w.Header().Set("Content-Type", "text/plain")
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_, err = w.Write(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing error response: %s\noriginal response: %s\n", err, string(body))
	}
}
