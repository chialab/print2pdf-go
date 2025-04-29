package main

import (
	"context"
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
			jsonError(w, "internal server error", http.StatusInternalServerError)

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
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	h, err := print2pdf.NewS3Handler(r.Context(), BucketName, data.FileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating print handler: %s\n", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	res, err := print2pdf.PrintPDF(r.Context(), data, h)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)
		jsonError(w, ve.Error(), http.StatusBadRequest)

		return
	} else if errors.Is(r.Context().Err(), context.Canceled) {
		fmt.Println("connection closed or request canceled")

		return
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF: %s\n", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	body, err := json.Marshal(ResponseData{Url: res})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding response to JSON: %s\n", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)

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
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", data.FileName))
	h := print2pdf.NewStreamHandler(w)
	_, err = print2pdf.PrintPDF(r.Context(), data, h)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)
		jsonError(w, ve.Error(), http.StatusBadRequest)
	} else if errors.Is(r.Context().Err(), context.Canceled) {
		fmt.Println("connection closed or request canceled")
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF: %s\n", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
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

	cookiesEnv := os.Getenv("COOKIES_TO_READ")
    cookieNames := strings.Split(cookiesEnv, ",")
    fmt.Printf("COOKIES_TO_READ: %s\n", cookieNames)

    foundCookies := ExtractCookies(r, cookieNames)
    
    if len(foundCookies) > 0 {
        data.Cookies = foundCookies
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

// jsonError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func jsonError(w http.ResponseWriter, message string, code int) {
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


// ExtractCookies extracts cookies from the request matching the specified names.
// Parameters:
// - r: the incoming HTTP request
// - names: list of cookie names to extract
// Returns:
// - a map with cookie names as keys and their values as map values
func ExtractCookies(r *http.Request, names []string) map[string]string {
	// Create a lookup map for desired cookie names
	wanted := make(map[string]bool)
	for _, name := range names {
		wanted[strings.TrimSpace(name)] = true
	}
	
	// Collect found cookies
	found := make(map[string]string)
	for _, c := range r.Cookies() {
		if wanted[c.Name] {
			found[c.Name] = c.Value
		}
	}

	return found
}