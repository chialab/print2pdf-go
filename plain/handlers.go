package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
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
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	origin := r.Header.Get("Origin")
	if allowOrigin, err := getCorsOriginHeader(origin); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	} else if allowOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
	}

	w.WriteHeader(http.StatusOK)
}

// Handle POST requests to "/v1/print" endpoint.
func handlePrintV1Post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	origin := r.Header.Get("Origin")
	if allowOrigin, err := getCorsOriginHeader(origin); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	} else if allowOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
	}

	data, err := readRequest(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}
	if err := checkPrintIsAllowed(data.Url); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "URL is not allowed", http.StatusForbidden)

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
	origin := r.Header.Get("Origin")
	if allowOrigin, err := getCorsOriginHeader(origin); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	} else if allowOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
	}

	data, err := readRequest(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "internal server error", http.StatusInternalServerError)

		return
	}
	if err := checkPrintIsAllowed(data.Url); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		jsonError(w, "URL is not allowed", http.StatusForbidden)

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

	data.Cookies = extractCookies(r.Cookies())

	return data, nil
}

// matchSlice checks if a string matches any one pattern in a list, case insensitive.
func matchSlice(patterns []string, s string) (bool, error) {
	for _, pattern := range patterns {
		pattern = fmt.Sprintf("^%s$", strings.ReplaceAll(regexp.QuoteMeta(strings.ToLower(pattern)), "\\*", ".*"))
		if matched, err := regexp.MatchString(pattern, strings.ToLower(s)); err != nil {
			return false, err
		} else if matched {
			return true, nil
		}
	}

	return false, nil
}

// getCorsOriginHeader gets the value for the CORS allow-origin header.
func getCorsOriginHeader(origin string) (string, error) {
	if CorsAllowedHosts == "" || CorsAllowedHosts == "*" {
		return "*", nil
	}

	allowedHosts := strings.Split(CorsAllowedHosts, ",")
	if matched, err := matchSlice(allowedHosts, origin); err != nil {
		return "", err
	} else if matched {
		return origin, nil
	}

	return "", nil
}

// checkPrintIsAllowed checks that printing the URL is allowed.
func checkPrintIsAllowed(u string) error {
	if PrintAllowedHosts == "" || PrintAllowedHosts == "*" {
		return nil
	}

	parsedUrl, err := url.Parse(u)
	if err != nil {
		return err
	}

	checkUrl := fmt.Sprintf("%s://%s", parsedUrl.Scheme, parsedUrl.Host)
	allowedHosts := strings.Split(PrintAllowedHosts, ",")
	if matched, err := matchSlice(allowedHosts, checkUrl); err != nil {
		return err
	} else if !matched {
		return fmt.Errorf("requested URL %s is not allowed for printing", u)
	}

	return nil
}

// extractCookies extracts cookies from the request matching the specified names.
// Parameters:
// - cookies: the request cookies
// Returns:
// - a map with cookie names as keys and their values as map values
func extractCookies(cookies []*http.Cookie) map[string]string {
	// Create a lookup map for desired cookie names
	names := strings.Split(ForwardCookies, ",")
	wanted := make(map[string]bool)
	for _, name := range names {
		wanted[strings.TrimSpace(name)] = true
	}

	// Collect found cookies
	found := make(map[string]string)
	for _, c := range cookies {
		if wanted[c.Name] {
			found[c.Name] = c.Value
		}
	}

	return found
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
