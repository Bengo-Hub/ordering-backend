APP_NAME := food-delivery-backend

.PHONY: run test lint tidy build swagger

run:
	go run ./cmd/api

swagger:
	go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g cmd/api/main.go -o internal/http/docs --parseDependency --parseInternal

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

build:
	CGO_ENABLED=0 go build -o bin/$(APP_NAME) ./cmd/api
