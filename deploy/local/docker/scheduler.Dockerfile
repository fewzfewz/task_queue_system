FROM golang:1.25-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /scheduler ./cmd/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /cli ./cmd/cli

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /scheduler /app/scheduler
COPY --from=builder /cli /app/cli
COPY db/migrations /app/db/migrations
USER appuser
EXPOSE 8082
CMD ["./scheduler"]
