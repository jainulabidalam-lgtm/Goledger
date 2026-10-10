# Build stage
FROM golang:1.27.2-alpine AS builder

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# modernc.org/sqlite is a pure Go driver, so we can safely disable CGO for a static binary.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o goledger ./cmd/goledger

# Runtime stage
FROM alpine:latest

# Add ca-certificates in case of outbound HTTPS requests
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the compiled binary from the builder
COPY --from=builder /app/goledger .

# Ensure the app binds to 0.0.0.0 (required by Render)
ENV HOST=0.0.0.0

# The PORT environment variable will be dynamically set by Render, 
# but we can document the default here.
EXPOSE 8080

# Run the application
CMD ["./goledger"]
