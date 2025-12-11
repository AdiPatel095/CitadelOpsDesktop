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

# Build the Go application into a static executable.
RUN CGO_ENABLED=0 GOOS=windows go build -a -ldflags '-s -w' -o /app/CitadelDesktop.exe .

# --- Stage 3: The Final Stage ---
FROM alpine:latest

# We need to add this for our app to run
RUN apk --no-cache add ca-certificates

# Set the working directory
WORKDIR /app

# Copy the compiled binary from the 'builder' stage
COPY --from=builder /app/CitadelDesktop.exe /app/CitadelDesktop.exe

# Expose port (optional, mostly for documentation)
EXPOSE 8080

# The command to run your application
ENTRYPOINT [ "/app/CitadelDesktop.exe" ]