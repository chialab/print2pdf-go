package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/chialab/print2pdf-go/print2pdf"
)

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

	// Read allowed cookie names
    cookiesEnv := os.Getenv("COOKIES_TO_READ")
    allowedCookies := make(map[string]struct{})
    for _, name := range strings.Split(cookiesEnv, ",") {
        allowedCookies[strings.TrimSpace(name)] = struct{}{}
    }

	var data print2pdf.GetPDFParams

	cookieHeader, ok := event.Headers["Cookie"]
    if ok {
        cookiePairs := strings.Split(cookieHeader, ";")
        for _, pair := range cookiePairs {
            parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
            if len(parts) == 2 {
                name := strings.TrimSpace(parts[0])
                value := strings.TrimSpace(parts[1])
                if _, allowed := allowedCookies[name]; allowed {
                    fmt.Printf("Adding cookie: %s = %s\n", name, value)
                    Aggiungi il cookie alla mappa
                    if data.Cookies == nil {
                        data.Cookies = make(map[string]string)
                    }
                    data.Cookies[name] = value
                }
            }
        }
    } else {
        fmt.Println("No 'Cookie' header found in request.")
    }

	err := json.Unmarshal([]byte(event.Body), &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error decoding JSON: %s\noriginal request body: %s\n", err, event.Body)

		return jsonError("internal server error", 500), nil
	}
	if !strings.HasSuffix(data.FileName, ".pdf") {
		data.FileName += ".pdf"
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
