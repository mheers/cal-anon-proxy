IMAGE   := mheers/cal-anon-proxy
TAG     := latest
REGISTRY := docker.io

.PHONY: all build push up down test

all: build

## build: build the Docker image
build:
	go build -o cal-anon-proxy .
	docker build -t $(IMAGE):$(TAG) .

## push: push the Docker image to the registry
push:
	docker push $(REGISTRY)/$(IMAGE):$(TAG)

## up: start services with docker compose
up:
	docker compose up -d

## down: stop services with docker compose
down:
	docker compose down

## test: run Go tests
test:
	go test -race ./...

## help: show this help
help:
	@grep -E '^##' Makefile | sed 's/## //'
