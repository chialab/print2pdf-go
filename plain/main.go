package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/chialab/print2pdf-go/print2pdf"
)

var Port = os.Getenv("PORT")
var BucketName = os.Getenv("BUCKET")
var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

func main() {
	http.HandleFunc("/print", printHandler)
	fmt.Printf("server listening on port %s", Port)
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
	go func() {
		switch r.Method {
		case "OPTIONS":
			handlePrintOptions(w, r)
		case "POST":
			handlePrintPost(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}()
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

	var requestData print2pdf.RequestData
	err := json.NewDecoder(r.Body).Decode(&requestData)
	if err != nil {
		JsonError(w, "Error reading request data", http.StatusInternalServerError)

		return
	}

	var buf []byte
	err = print2pdf.GetPDFBuffer(r.Context(), requestData, &buf)
	if e, ok := err.(print2pdf.ValidationError); ok {
		JsonError(w, e.Error(), http.StatusBadRequest)

		return
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF buffer: %s", err)
		JsonError(w, "Internal server error", http.StatusInternalServerError)

		return
	}

	url, err := print2pdf.UploadFile(r.Context(), BucketName, requestData.FileName, &buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading file: %s", err)
		JsonError(w, "Internal server error", http.StatusInternalServerError)

		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(print2pdf.ResponseData{Url: url})
}

type ErrorResponse struct {
	Message string `json:"message"`
}

// JsonError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func JsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{message})
}
