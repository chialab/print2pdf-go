# Print2PDF

[![Go package release](https://img.shields.io/github/v/tag/chialab/print2pdf-go?filter=print2pdf%2F*&logo=go&logoColor=white&label=pkg&color=007d9c)](https://pkg.go.dev/github.com/chialab/print2pdf-go/print2pdf)
[![Latest release](https://img.shields.io/github/v/release/chialab/print2pdf-go?logo=github&logoColor=white)](https://github.com/chialab/print2pdf-go/releases/latest)

A tool to print webpages as PDF files, powered by [chromedp](https://github.com/chromedp/chromedp) and the [DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/).

## Go package

Install with:
```shell
go get github.com/chialab/print2pdf-go/print2pdf
```

and use it in your project. See the [package reference](https://pkg.go.dev/github.com/chialab/print2pdf-go/print2pdf)
or the [plain](plain) application for an example.

## REST application

The `plain` directory in this repository contains a minimal REST application. It is also provided as a
binary for each [release](https://github.com/chialab/print2pdf-go/releases/latest).

The following environmental variables can be used to configure the application:
- `CHROMIUM_PATH` (**required**) full path to the Chromium binary
- `BUCKET` (**required** by endpoint `/v1/print` and `lambda` application) name of the AWS S3 bucket where to store the generated PDF
- `PORT` (**optional**, default to `3000`) port from which the `plain` application will be served
- `CORS_ALLOWED_HOSTS` (**optional**, default to `*`) comma-separated list of allowed origins for pre-flight CORS requests

To use the `/v1/print` endpoint, credentials for the AWS account need to be configured in your environment to be able to store
the generated PDF in AWS S3. See the [SDK documentation](https://aws.github.io/aws-sdk-go-v2/docs/configuring-sdk/#specifying-credentials)
for the supported methods of providing the credentials.

Launch the binary to start the webserver at `http://localhost:3000`.

The webserver provides three endpoints:
- `/v1/print` stores the generated PDF in an AWS S3 bucket
- `/v2/print` streams the generated PDF as the response
- `/metrics` exports metrics in the Prometheus format

Both endpoints accept `POST` requests with the following body parameters:
- `url` (**required**) the URL of the page to print as PDF
- `file_name` (**required**) the filename of the exported PDF; the suffix `.pdf` can be omitted and will be
                             automatically added if missing
- `media` (**optional**) the media type to emulate when printing the PDF; can be either `print` or `screen`,
                         default is `print`
- `format` (**optional**) the printed PDF page format; can be one of `Letter`, `Legal`, `Tabloid`, `Ledger`, `A0`,
                          `A1`, `A2`, `A3`, `A4`, `A5` or `A6`, default is `A4`
- `background` (**optional**) whether to print background graphics; can be either `true` or `false`, default is `true`
- `layout` (**optional**) page orientation of the printed PDF; can be either `landscape` or `portrait`, default
                          is `portrait`
- `margins` (**optional**) page margins of the printed PDF; is an object with four optional keys `top`, `bottom`,
                           `left` and `right` expressed in inches as decimal numbers, the default for each one is 0
- `scale` (**optional**) print scale; is a positive decimal number, default is 1 (meaning 100%)

The `/v1/print` endpoint responds with a JSON object with the key `url` containing the URL to the file, while the `/v2/print`
endpoint is the file itself. In case of an error the response will have an appropriate HTTP status code and its body will be a JSON
object with the key `message` explaining the error, and a log line will be written to the console with more details.

## Lambda function

The `lambda` directory in this repository contains a lambda function, to be run on AWS Lambda. It is also provided as a
binary for each [release](https://github.com/chialab/print2pdf-go/releases/latest).

It provides the same endpoint as the `/v1/print` endpoint of the [REST application](#rest-application), but is expected to
be run behind an API Gateway so the request body [is a bit different](https://docs.aws.amazon.com/apigateway/latest/developerguide/set-up-lambda-proxy-integrations.html#api-gateway-simple-proxy-for-lambda-input-format).

To use it locally and in the cloud, see the docker image usage.

## Docker image

Docker images for the `plain` and `lambda` applications are provided. They both come with Chromium pre-installed.

The `plain` image can be used like this:
```shell
docker run --rm -it -p '3000:3000' -e 'BUCKET=mybucket' -e 'CHROMIUM_PATH=/usr/bin/chromium' ghcr.io/chialab/print2pdf-go/plain:latest
```

The `lambda` image can be used locally like this:
```shell
docker run --rm -it -p '8080:8080' -e 'BUCKET=mybucket' -e 'CHROMIUM_PATH=/usr/bin/chromium' --entrypoint '/usr/local/bin/aws-lambda-rie' ghcr.io/chialab/print2pdf-go/lambda:latest "/app/print2pdf"
```

The image is based on [lambda/provided](https://gallery.ecr.aws/lambda/provided), so it comes with [Lambda RIE](https://github.com/aws/aws-lambda-runtime-interface-emulator/)
pre-packaged. See [AWS documentation](https://docs.aws.amazon.com/prescriptive-guidance/latest/patterns/deploy-lambda-functions-with-container-images.html)
for more informations on how to deploy Lambda functions using container images.

Since it is expected to be run behind an API Gateway, to be used locally the request body must be converted to a JSON string
and used as value of a `body` parameter. Also, the actual endpoint to call is `http://localhost:8080/2015-03-31/functions/function/invocations`.

### Terraform module

A Terraform module is provided in the `terraform` directory for convenience, it can be used to setup the AWS infrastructure
needed to deploy the image as a Lambda function:
```terraform
module "lambda" {
    source = "git::ssh://git@github.com:chialab/print2pdf-go.git//terraform"
    ...
}
```

**NOTE:** you will need to build and push the Docker image to the created ECR repository before creating the Lambda function.

### Helm chart

An Helm chart is provided in the `chart` directory for deploying the `plain` application in Kubernetes, and is distributed using GitHub's OCI container registry.

The chart repo URL is `oci://ghcr.io/chialab/helm-charts/print2pdf-go`. Usage example:
```shell
helm install example-release oci://ghcr.io/chialab/helm-charts/print2pdf-go --namespace example-ns --values example.yml --version ~0.1.0
```
