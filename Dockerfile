# Build stage
FROM golang:1.22-alpine AS builder

# Set the working directory
WORKDIR /app

# Install git and ca-certificates (needed for fetching dependencies and HTTPS calls)
RUN apk add --no-cache git ca-certificates

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the Go application statically
# CGO_ENABLED=0 ensures a static binary, crucial for the scratch image
# -ldflags="-s -w" removes debugging information to reduce the binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server/main.go

# Final minimal stage
FROM scratch

# Copy root certificates so HTTPS requests in your service don't fail
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the static binary from the builder stage
COPY --from=builder /app/server /server

# Expose the port (informative, actual binding comes from env)
EXPOSE 8080

# Command to run the executable
ENTRYPOINT ["/server"]
