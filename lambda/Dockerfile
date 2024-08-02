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
# Download chromium for AWS Lambda
###
FROM --platform=$BUILDPLATFORM alpine:3 AS downloader

WORKDIR /app

ADD https://github.com/Sparticuz/chromium/releases/download/v126.0.0/chromium-v126.0.0-pack.tar /app/chromium-pack.tar
RUN tar -xf chromium-pack.tar \
 && rm chromium-pack.tar

###
# Final image
###
FROM public.ecr.aws/lambda/provided:al2023

WORKDIR /app

COPY --from=downloader /app/al2023.tar.br /app/fonts.tar.br /app/swiftshader.tar.br /app/chromium.br /tmp/
RUN dnf -y install tar brotli \
 && mkdir -p /tmp/al2023 /tmp/fonts /tmp/swiftshader \
 && { brotli --decompress --rm --stdout /tmp/al2023.tar.br | tar -C /tmp/al2023/ -xf - ; } \
 && { brotli --decompress --rm --stdout /tmp/fonts.tar.br | tar -C /tmp/fonts/ -xf - ; } \
 && { brotli --decompress --rm --stdout /tmp/swiftshader.tar.br | tar -C /tmp/swiftshader -xf - ; } \
 && brotli --decompress --rm --output=/tmp/chromium /tmp/chromium.br \
 && chmod +x /tmp/chromium \
 && dnf -y remove tar brotli \
 && dnf clean all

ENV FONTCONFIG_PATH="/tmp/fonts" \
    LD_LIBRARY_PATH="/tmp/al2023/lib:/tmp/swiftshader:${LD_LIBRARY_PATH}" \
    VK_ICD_FILENAMES="/tmp/swiftshader/vk_swiftshader_icd.json"

COPY --from=builder /app/build/print2pdf /app/

ENTRYPOINT ["/app/print2pdf"]
