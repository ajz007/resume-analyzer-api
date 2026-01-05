# ---------- Build stage ----------
# Use a Go toolchain that satisfies go.mod (>= 1.24.1)
FROM golang:1.24.1-alpine AS builder

WORKDIR /app

# Install git (needed for go modules sometimes)
RUN apk add --no-cache git

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o resume-analyzer-api ./cmd/api

# ---------- Runtime stage ----------
FROM gcr.io/distroless/base-debian12

WORKDIR /app

# Copy binary
COPY --from=builder /app/resume-analyzer-api /app/resume-analyzer-api

# Expose API port
EXPOSE 8080

# Run
ENTRYPOINT ["/app/resume-analyzer-api"]
