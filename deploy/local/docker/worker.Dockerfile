FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /scheduler ./cmd/scheduler

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /worker /app/worker
COPY --from=builder /scheduler /app/scheduler
CMD ["./worker"]
