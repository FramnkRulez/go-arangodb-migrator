# Multi-stage build for ArangoDB Migrator CLI
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the CLI tool
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o arangodb-migrator ./cmd/migrator

# Final stage - minimal runtime image
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S arangodb && \
    adduser -u 1001 -S arangodb -G arangodb

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/arangodb-migrator .

# Change ownership to non-root user
RUN chown arangodb:arangodb arangodb-migrator

# Switch to non-root user
USER arangodb

# Set entrypoint
ENTRYPOINT ["./arangodb-migrator"]

# Default command (can be overridden)
CMD ["--help"] 