.PHONY: bin-plain bin-lambda docker-plain docker-lambda

IMAGE_NAME ?= print2pdf
IMAGE_TAG ?= dev

bin-plain:
	CGO_ENABLED=0 go build -ldflags '-s' -o build/print2pdf-plain plain

bin-lambda:
	CGO_ENABLED=0 go build -ldflags '-s' -o build/print2pdf-lambda lambda

docker-plain:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file plain/Dockerfile plain/

docker-lambda:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file lambda/Dockerfile lambda/
