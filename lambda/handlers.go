package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/chialab/print2pdf-go/print2pdf"
)

// Handle a request.
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	origin, ok := event.Headers["Origin"]
	if !ok {
		origin = event.Headers["origin"]
	}
	if allowOrigin, err := getCorsOriginHeader(origin); err != nil {
		fmt.Fprintf(os.Stderr, "error getting CORS allow-origin header value: %s\n", err)

		return jsonError("internal server error", 500), nil
	} else if allowOrigin != "" {
		headers["Access-Control-Allow-Origin"] = origin
	}

	var data print2pdf.GetPDFParams
	err := json.Unmarshal([]byte(event.Body), &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error decoding JSON: %s\noriginal request body: %s\n", err, event.Body)

		return jsonError("internal server error", 500), nil
	}
	if !strings.HasSuffix(data.FileName, ".pdf") {
		data.FileName += ".pdf"
	}
	if err := checkPrintIsAllowed(data.Url); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())

		return jsonError("URL is not allowed", 403), nil
	}

	cookieHeader, ok := event.Headers["Cookie"]
	if !ok {
		cookieHeader, ok = event.Headers["cookie"]
	}
	if ok {
		cookies := strings.Split(cookieHeader, ";")
		data.Cookies = extractCookies(cookies)
	}
	h, err := print2pdf.NewS3Handler(ctx, BucketName, data.FileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating print handler: %s\n", err)

		return jsonError("internal server error", 500), nil
	}

	url, err := print2pdf.PrintPDF(ctx, data, h)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)

		return jsonError(ve.Error(), 400), nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF: %s\n", err)

		return jsonError("internal server error", 500), nil
	}

	body, err := json.Marshal(ResponseData{Url: url})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding response to JSON: %s\n", err)

		return jsonError("internal server error", 500), nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(body),
		Headers:    headers,
	}, nil
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
func extractCookies(cookies []string) map[string]string {
	// Create a lookup map for desired cookie names
	names := strings.Split(ForwardCookies, ",")
	wanted := make(map[string]bool)
	for _, name := range names {
		wanted[strings.TrimSpace(name)] = true
	}

	// Collect found cookies
	forward := make(map[string]string)
	for _, cookie := range cookies {
		name, value, found := strings.Cut(strings.TrimSpace(cookie), "=")
		if !found {
			continue
		}

		name = strings.TrimSpace(name)
		if wanted[name] {
			forward[name] = strings.TrimSpace(value)
		}
	}

	return forward
}

// Prepare an HTTP error response.
func jsonError(message string, code int) events.APIGatewayProxyResponse {
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
