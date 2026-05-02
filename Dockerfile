FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies first to cache this layer
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build both binaries statically
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /worker ./cmd/worker

# Final lightweight stage
FROM alpine:latest

WORKDIR /app

# Add tzdata for logging time accurately
RUN apk add --no-cache tzdata ca-certificates

# Copy from builder
COPY --from=builder /api /app/api
COPY --from=builder /worker /app/worker

# Running as a non-root user for security
RUN adduser -D appuser
USER appuser

# Entrypoint will be overridden by docker-compose to choose either api or worker
