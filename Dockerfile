# ============================================================
# Stage 1: Build the React frontend
# ============================================================
FROM node:20-alpine AS frontend

WORKDIR /build

# Install dependencies first (better caching)
COPY package.json package-lock.json* ./
RUN npm install

# Copy source and config files needed for the build
COPY src/ src/
COPY public/ public/
COPY index.html vite.config.ts postcss.config.mjs* ./
RUN npm run build

# ============================================================
# Stage 2: Build the Go backend
# ============================================================
FROM golang:1.22-alpine AS backend

WORKDIR /build

# Install git (needed for go mod download of some deps)
RUN apk add --no-cache git

# Copy go module files and download dependencies
COPY backend/go.mod backend/go.sum* ./
RUN go mod tidy && go mod download

# Copy backend source and build a static binary
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o orexa-panel .

# ============================================================
# Stage 3: Runtime image with Java for Minecraft servers
# ============================================================
FROM eclipse-temurin:21-jre-jammy

LABEL maintainer="Orexa Panel"
LABEL description="Minecraft server administration panel"

# Install tar + gzip (for backups), git (for Spigot BuildTools), gosu (for privilege drop), and basic utilities
RUN apt-get update && \
    apt-get install -y --no-install-recommends tar gzip curl git gosu ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user for running the panel and Minecraft servers
RUN groupadd -r mcpanel && useradd -r -g mcpanel -m -s /bin/bash mcpanel

# Create the application directory structure and set ownership
RUN mkdir -p /AdPanel/Servers /AdPanel/data /AdPanel/dist /AdPanel/Backups && \
    chown -R mcpanel:mcpanel /AdPanel

# Copy the compiled Go binary
COPY --from=backend --chown=mcpanel:mcpanel /build/orexa-panel /AdPanel/orexa-panel

# Copy the built React frontend
COPY --from=frontend --chown=mcpanel:mcpanel /build/dist /AdPanel/dist

# Copy entrypoint script (runs as root to fix volume ownership, then drops to mcpanel)
COPY entrypoint.sh /AdPanel/entrypoint.sh
RUN chmod +x /AdPanel/entrypoint.sh

WORKDIR /AdPanel

# Web UI port
EXPOSE 4010

# Default Minecraft server port range (map as needed)
EXPOSE 25565-25575

# Persistent data
VOLUME ["/AdPanel/Servers", "/AdPanel/data", "/AdPanel/Backups"]

# Environment variable for the base directory (already defaults to /AdPanel in the binary)
ENV ADPANEL_DIR=/AdPanel

# Entrypoint fixes volume ownership then runs as mcpanel via gosu
ENTRYPOINT ["/AdPanel/entrypoint.sh"]

