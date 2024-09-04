.PHONY: bin-plain bin-lambda docker-plain docker-lambda

IMAGE_NAME ?= print2pdf
IMAGE_TAG ?= dev

bin-plain:
	CGO_ENABLED=0 go build -ldflags '-s' -o build/print2pdf-plain plain/main.go

bin-lambda:
	CGO_ENABLED=0 go build -ldflags '-s' -tags 'lambda.norpc' -o build/print2pdf-lambda lambda/main.go

docker-plain:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file plain/Dockerfile plain/

docker-lambda:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file lambda/Dockerfile lambda/
