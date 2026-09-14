# Build Stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install ca-certificates and tzdata
RUN apk add --no-cache ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o spectra .

# Runtime Stage (Ultra-lightweight)
FROM alpine:3.20

WORKDIR /app

# Install CA certificates and tzdata for accurate time/HTTPS
RUN apk add --no-cache ca-certificates tzdata

# Copy compiled binary from builder
COPY --from=builder /app/spectra /app/spectra

# Default port
ENV PORT=5050
EXPOSE 5050

# Container healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:5050/health || exit 1

# Run Spectra
ENTRYPOINT ["/app/spectra"]
