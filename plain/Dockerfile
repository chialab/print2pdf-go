###
# Build binary
###
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum /app/
RUN go mod download

COPY *.go /app/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags '-s' -tags 'lambda.norpc' -o build/print2pdf

###
# Final image
###
FROM chromedp/headless-shell:stable

WORKDIR /app

COPY --from=builder /app/build/print2pdf /app/

ENTRYPOINT ["/app/print2pdf"]
