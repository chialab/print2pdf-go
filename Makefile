.PHONY: lambda

IMAGE_NAME ?= print2pdf
IMAGE_TAG ?= dev

lambda:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file lambda/Dockerfile lambda/
