package main

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/chialab/print2pdf-go/print2pdf"
)

var BucketName = os.Getenv("BUCKET")
var CorsAllowedHosts = os.Getenv("CORS_ALLOWED_HOSTS")

// Handle a request.
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	responseHeaders := map[string]string{"Content-Type": "application/json"}
	origin, ok := event.Headers["Origin"]
	if !ok {
		origin = event.Headers["origin"]
	}
	if CorsAllowedHosts != "" && origin != "" {
		if CorsAllowedHosts == "*" {
			responseHeaders["Access-Control-Allow-Origin"] = "*"
		} else {
			allowedHosts := strings.Split(CorsAllowedHosts, ",")
			if slices.Contains(allowedHosts, origin) {
				responseHeaders["Access-Control-Allow-Origin"] = origin
			}
		}
	}

	var requestData print2pdf.RequestData
	err := json.Unmarshal([]byte(event.Body), &requestData)
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}

	var buf []byte
	err = print2pdf.GetPDFBuffer(ctx, requestData, &buf)
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}

	url, err := print2pdf.UploadFile(ctx, BucketName, requestData.FileName, &buf)
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}

	responseData := print2pdf.ResponseData{Url: url}
	responseJson, err := json.Marshal(responseData)
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       string(responseJson),
		Headers:    responseHeaders,
	}, nil
}

func main() {
	lambda.Start(handler)
}
