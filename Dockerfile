# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# Install certificates for HTTPS calls during go get
RUN apk add --no-cache ca-certificates git

WORKDIR /app

# Cache module downloads separately from source code
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically-linked binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/server ./cmd/server

# Default command for the builder stage (used by docker-compose)
CMD ["/app/server"]

# ── Stage 2: Minimal runtime image ──────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy compiled binaries from the builder stage
COPY --from=builder /app/server ./server
COPY --from=builder /app/seed   ./seed

EXPOSE 5002

CMD ["./server"]
