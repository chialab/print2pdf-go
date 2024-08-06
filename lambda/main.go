package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chialab/print2pdf-go/print2pdf"
)

type ResponseData struct {
	Url string `json:"url"`
}

type ResponseError struct {
	Message string `json:"message"`
}

var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

func main() {
	lambda.Start(handler)
}

// Handle a request.
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	origin, ok := event.Headers["Origin"]
	if !ok {
		origin = event.Headers["origin"]
	}
	if CorsAllowedHosts != "" && origin != "" {
		if CorsAllowedHosts == "*" {
			headers["Access-Control-Allow-Origin"] = "*"
		} else {
			hosts := strings.Split(CorsAllowedHosts, ",")
			if slices.Contains(hosts, origin) {
				headers["Access-Control-Allow-Origin"] = origin
			}
		}
	}

	var data print2pdf.GetPDFParams
	err := json.Unmarshal([]byte(event.Body), &data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error decoding JSON: %s\noriginal request body: %s\n", err, event.Body)

		return JsonError("internal server error", 500), nil
	}

	var buf []byte
	err = print2pdf.GetPDFBuffer(ctx, data, &buf)
	if ve, ok := err.(print2pdf.ValidationError); ok {
		fmt.Fprintf(os.Stderr, "request validation error: %s\n", ve)

		return JsonError(ve.Error(), 400), nil
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "error getting PDF buffer: %s\n", err)

		return JsonError("internal server error", 500), nil
	}

	url, err := print2pdf.UploadFile(ctx, data.FileName, &buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error uploading file: %s\n", err)

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
