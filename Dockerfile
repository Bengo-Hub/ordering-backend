# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./

RUN GOTOOLCHAIN=auto go mod download
COPY . .
# Build all binaries: api, migrate, and seed
RUN GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ordering-backend ./cmd/api && \
    GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ordering-migrate ./cmd/migrate && \
    GOTOOLCHAIN=auto CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/ordering-seed ./cmd/seed

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && addgroup -S app && adduser -S app -G app
WORKDIR /app
# Copy all binaries to well-known locations
COPY --from=builder /bin/ordering-backend /usr/local/bin/ordering-backend
COPY --from=builder /bin/ordering-migrate /usr/local/bin/ordering-migrate
COPY --from=builder /bin/ordering-seed /usr/local/bin/ordering-seed
COPY internal/ent/migrate/migrations ./internal/ent/migrate/migrations
# Media directory is optional (populated at runtime via PVC mount)
RUN mkdir -p ./media
COPY scripts/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
USER app
EXPOSE 4000
ENV PORT=4000
# Use entrypoint script to wait for DB before starting (matches auth-api pattern)
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
