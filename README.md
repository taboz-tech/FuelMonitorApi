# Fuel Monitor API - Go Version

A high-performance Go API for the Fuel Monitor application, migrated from Node.js for improved response times and resource efficiency.

## Features

- JWT-based authentication
- SSH tunnel support for remote database connections
- PostgreSQL integration with connection pooling
- CORS support for frontend integration
- Docker containerization
- Graceful shutdown handling
- Health check endpoints

## Quick Start

### Prerequisites

- Go 1.21 or later
- Docker and Docker Compose
- Access to the PostgreSQL database (via SSH tunnel)

### Environment Setup

1. Copy the environment file:
   ```bash
   cp .env.example .env
   ```

2. Update the `.env` file with your database and SSH credentials.

### Running with Docker (Recommended)

1. Build and run with Docker Compose:
   ```bash
   make docker-up
   ```

2. The API will be available at `http://localhost:4174`

3. Check logs:
   ```bash
   make docker-logs
   ```

4. Stop the application:
   ```bash
   make docker-down
   ```

### Running Locally

1. Install dependencies:
   ```bash
   make deps
   ```

2. Run the application:
   ```bash
   make run
   ```

### Development

- Build the binary: `make build`
- Run tests: `make test`
- Format code: `make fmt`
- Clean build files: `make clean`

## API Endpoints

### Authentication

- `POST /api/auth/login` - User login
- `POST /api/auth/logout` - User logout (requires authentication)
- `GET /api/auth/validate` - Validate JWT token (requires authentication)

### Health Check

- `GET /api/health` - Health check endpoint

## Authentication

The API uses JWT tokens for authentication. Include the token in the Authorization header:

```
Authorization: Bearer <your_jwt_token>
```

### Login Example

```bash
curl -X POST http://localhost:4174/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "admin123"
  }'
```

Response:
```json
{
  "user": {
    "id": 1,
    "username": "admin",
    "email": "admin@fuelmonitor.com",
    "role": "admin",
    "fullName": "System Administrator",
    "isActive": true,
    "lastLogin": "2024-01-01T12:00:00Z",
    "createdAt": "2024-01-01T10:00:00Z"
  },
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

## Database Connection

The application connects to PostgreSQL through an SSH tunnel:

1. SSH connection is established to the remote server
2. Local tunnel is created (random port)
3. Database connection uses the local tunnel port
4. All database operations go through the encrypted tunnel

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | API server port | 4174 |
| `SSH_HOST` | SSH server hostname | - |
| `SSH_USERNAME` | SSH username | - |
| `SSH_PASSWORD` | SSH password | - |
| `REMOTE_BIND_HOST` | Remote database host | 127.0.0.1 |
| `REMOTE_BIND_PORT` | Remote database port | 5437 |
| `DB_NAME` | Database name | sensorsdb |
| `DB_USER` | Database username | sa |
| `DB_PASSWORD` | Database password | - |
| `JWT_SECRET` | JWT signing secret | - |
| `GIN_MODE` | Gin mode (debug/release) | debug |

## Docker Configuration

The Dockerfile uses multi-stage builds for optimized image size:
- Build stage: Full Go environment for compilation
- Final stage: Minimal Alpine Linux with only runtime dependencies

## Performance Improvements

Compared to the Node.js version:
- **Faster startup time**: Go compiles to native machine code
- **Lower memory usage**: More efficient garbage collection
- **Better concurrency**: Native goroutines for handling multiple requests
- **Connection pooling**: Optimized database connection management
- **Smaller container size**: Alpine-based final image

## Security

- JWT tokens expire after 24 hours
- Passwords are hashed using bcrypt
- SSH tunnel provides encrypted database connection
- CORS configuration restricts allowed origins

## Monitoring

- Health check endpoint for load balancer integration
- Structured logging with request/response details
- Graceful shutdown handling for zero-downtime deployments

## Migration from Node.js

This Go version maintains API compatibility with the original Node.js version:
- Same endpoint URLs and HTTP methods
- Identical request/response formats
- Same authentication mechanism
- Compatible with existing frontend applications

## Troubleshooting

### Common Issues

1. **SSH Connection Failed**
   - Verify SSH credentials in `.env`
   - Check firewall settings
   - Ensure SSH server is accessible

2. **Database Connection Failed**
   - Verify database credentials
   - Check if SSH tunnel is working
   - Ensure PostgreSQL is running

3. **Port Already in Use**
   - Change the `PORT` environment variable
   - Check for other applications using port 4174

### Logs

View application logs:
```bash
# Docker Compose
make docker-logs

# Local development
./main 2>&1 | tail -f
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Format code: `make fmt`
6. Submit a pull request

## License

This project is licensed under the MIT License.