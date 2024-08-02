.PHONY: lambda plain

IMAGE_NAME ?= print2pdf
IMAGE_TAG ?= dev

plain:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file plain/Dockerfile plain/

lambda:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) --file lambda/Dockerfile lambda/
