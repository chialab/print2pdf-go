###
# Build binary
###
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=development

WORKDIR /app

COPY go.mod go.sum /app/
RUN go mod download

COPY *.go /app/
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-s -X main.Version=${VERSION}" -o build/print2pdf

###
# Final image
###
FROM chromedp/headless-shell:stable

WORKDIR /app

ENV CHROMIUM_PATH="/headless-shell/headless-shell"

RUN apt-get update \
 && apt-get install -y curl \
 && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/build/print2pdf /app/

ENTRYPOINT ["/app/print2pdf"]

HEALTHCHECK CMD curl -sf http://localhost:${PORT:-3000}/status
