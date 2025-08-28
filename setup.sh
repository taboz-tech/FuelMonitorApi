#!/bin/bash

# Setup script for Fuel Monitor API Go version

set -e  # Exit on any error

echo "🚀 Setting up Fuel Monitor API (Go version)..."

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.21"
if ! printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V -C; then
    echo "❌ Go version $GO_VERSION is too old. Please install Go $REQUIRED_VERSION or later."
    exit 1
fi

echo "✅ Go version $GO_VERSION detected"

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker."
    exit 1
fi

# Check if Docker Compose is available
if ! (command -v docker-compose &> /dev/null || docker compose version &> /dev/null); then
    echo "❌ Docker Compose is not installed. Please install Docker Compose."
    exit 1
fi

echo "✅ Docker and Docker Compose detected"

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file..."
    cat > .env << 'EOL'
# SSH Tunnel Configuration for External Database
SSH_HOST=41.191.232.15
SSH_USERNAME=sa
SSH_PASSWORD=s3rv3r5mx$
REMOTE_BIND_HOST=127.0.0.1
REMOTE_BIND_PORT=5437

# Database Configuration
DB_NAME=sensorsdb
DB_USER=sa
DB_PASSWORD=s3rv3r5mxdb
DB_HOST=127.0.0.1
DB_PORT=5432

# API Configuration
JWT_SECRET=c8f2e1e8d65f6fa3eff0d06ca3377dcd6a6918284f522ff70eba7eabea997994
PORT=4174
GIN_MODE=release
EOL
    echo "✅ .env file created"
else
    echo "✅ .env file already exists"
fi

# Initialize Go module if not exists
if [ ! -f go.mod ]; then
    echo "📦 Initializing Go module..."
    go mod init fuel-monitor-api
fi

# Download dependencies
echo "📦 Downloading Go dependencies..."
go mod tidy

# Create necessary directories
echo "📁 Creating directory structure..."
mkdir -p cmd/api
mkdir -p internal/{config,database,handlers,middleware,models,ssh}
mkdir -p scripts
mkdir -p docs

# Make the setup script executable
chmod +x setup.sh

echo ""
echo "🎉 Setup completed successfully!"
echo ""
echo "Next steps:"
echo "1. Review and update the .env file with your actual credentials"
echo "2. Run the application:"
echo "   - With Docker: make docker-up"
echo "   - Locally: make run"
echo ""
echo "The API will be available at: http://localhost:4174"
echo ""
echo "Available make commands:"
echo "  make help          - Show all available commands"
echo "  make docker-up     - Run with Docker Compose"
echo "  make run           - Run locally"
echo "  make test          - Run tests"
echo "  make build         - Build binary"
echo ""
echo "Happy coding! 🚀"
