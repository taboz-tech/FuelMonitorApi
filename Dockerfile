FROM golang:1.21-alpine AS builder

# Install git and openssh for dependencies and SSH tunneling
RUN apk add --no-cache git openssh-client

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/api

# Final stage
FROM alpine:latest

# Install openssh for SSH tunneling and ca-certificates for HTTPS
RUN apk --no-cache add openssh-client ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/main .

# Expose port 4174
EXPOSE 4174

# Run the application
CMD ["./main"]