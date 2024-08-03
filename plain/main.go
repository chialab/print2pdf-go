package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/chialab/print2pdf-go/print2pdf"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

var Port = os.Getenv("PORT")
var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

func main() {
	if Port == "" {
		Port = "3000"
	}

	http.HandleFunc("/print", printHandler)
	fmt.Printf("server listening on port %s\n", Port)
	err := http.ListenAndServe(":"+Port, nil)
	if errors.Is(err, http.ErrServerClosed) {
		fmt.Println("server closed")
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "server error: %s\n", err)
		os.Exit(1)
	}
}

// Handle requests to "/print" endpoint.
func printHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		handlePrintOptions(w, r)
	case "POST":
		handlePrintPost(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Handle OPTIONS requests to "/print" endpoint.
func handlePrintOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Methods", "OPTIONS,POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if CorsAllowedHosts == "" || CorsAllowedHosts == "*" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		allowedHosts := strings.Split(CorsAllowedHosts, ",")
		origin := r.Header.Get("Origin")
		if slices.Contains(allowedHosts, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// Handle POST requests to "/print" endpoint.
func handlePrintPost(w http.ResponseWriter, r *http.Request) {
	r.Context()
	w.Header().Set("Content-Type", "application/json")

	if CorsAllowedHosts == "" || CorsAllowedHosts == "*" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	} else {
		allowedHosts := strings.Split(CorsAllowedHosts, ",")
		origin := r.Header.Get("Origin")
		if slices.Contains(allowedHosts, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading request data: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	var data print2pdf.RequestData
	err = json.Unmarshal(body, &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error decoding JSON: %s\noriginal request body: %s\n", err, string(body))
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	var buf []byte
	err = print2pdf.GetPDFBuffer(r.Context(), data, &buf)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)
		JsonError(w, ve.Error(), http.StatusBadRequest)

		return
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF buffer: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	url, err := print2pdf.UploadFile(r.Context(), data.FileName, &buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading file: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	body, err = json.Marshal(print2pdf.ResponseData{Url: url})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding response to JSON: %s\n", err)
		JsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error writing response: %s\n", err)
	}
}

// JsonError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func JsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	body, err := json.Marshal(ErrorResponse{message})
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
