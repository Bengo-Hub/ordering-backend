# syntax=docker/dockerfile:1

FROM golang:1.23-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
RUN go mod download
COPY . .
# Build all binaries: api, migrate, and seed
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/api && \
    CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -o /out/seed ./cmd/seed

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /out/app /app/service
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /out/seed /app/seed
USER nonroot:nonroot
EXPOSE 4000
ENV PORT=4000
ENTRYPOINT ["/app/service"]
