# syntax=docker/dockerfile:1

# Build from monorepo root context (../../ from cafe-backend)
FROM golang:1.24-bookworm AS builder
WORKDIR /workspace

# Copy shared dependencies required by go.mod replace directive
COPY shared/auth-client ./shared/auth-client

# Copy cafe-backend 
COPY Cafe/cafe-backend ./Cafe/cafe-backend
WORKDIR /workspace/Cafe/cafe-backend

# Download dependencies (replace directive will use../../ shared/auth-client -> /workspace/shared/auth-client)
RUN go mod download

# Build all binaries
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
