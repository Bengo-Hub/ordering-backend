APP_NAME := food-delivery-backend

.PHONY: run test lint tidy build

run:
	go run ./cmd/api

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

build:
	CGO_ENABLED=0 go build -o bin/$(APP_NAME) ./cmd/api
