# --- Stage 1: Frontend Build Stage ---
FROM node:20-alpine AS frontend-builder
WORKDIR /app/Client
COPY Client/package.json Client/package-lock.json ./
RUN npm ci
COPY Client/ .
RUN npm run build

# --- Stage 2: Backend Build Stage ---
# Using golang:1.24-alpine
FROM golang:1.25-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy the module files first.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of your application's source code
COPY . .

# Copy the built frontend assets from the frontend-builder stage
# The path must match what //go:embed expects (Client/dist)
COPY --from=frontend-builder /app/Client/dist ./Client/dist

# Build the Go application into static executables for all platforms
# Windows (x64)
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags desktop -a -ldflags '-s -w' -o /app/CitadelDesktop.exe .
# macOS Intel (x64)
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -tags desktop -a -ldflags '-s -w' -o /app/CitadelDesktop-macos-amd64 .
# macOS Apple Silicon (ARM64)
RUN CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -tags desktop -a -ldflags '-s -w' -o /app/CitadelDesktop-macos-arm64 .

# --- Stage 3: The Final Stage ---
FROM alpine:latest

# We need to add this for our app to run
RUN apk --no-cache add ca-certificates

# Set the working directory
WORKDIR /app

# Copy the compiled binaries from the 'builder' stage
COPY --from=builder /app/CitadelDesktop.exe /app/CitadelDesktop.exe
COPY --from=builder /app/CitadelDesktop-macos-amd64 /app/CitadelDesktop-macos-amd64
COPY --from=builder /app/CitadelDesktop-macos-arm64 /app/CitadelDesktop-macos-arm64

# Expose port (optional, mostly for documentation)
EXPOSE 8080

# The command to run your application
ENTRYPOINT [ "/app/CitadelDesktop.exe" ]
