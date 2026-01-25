# =============================================================================
# Stage 1: Generate TypeScript Types
# =============================================================================
FROM golang:1.25.5-alpine AS typegen

WORKDIR /app

# Install make and yq (needed for Makefile)
RUN apk add --no-cache make yq

# Copy Go modules, tools definition, and Makefile
COPY go.mod go.sum tools.go Makefile ./
RUN go mod download

# Install tygo using version from go.mod/go.sum
COPY tygo.yaml ./
RUN make SHELL=/bin/ash ./build/api/tygo

# Copy source files needed for type generation
COPY pkg/ ./pkg/

# Generate TypeScript types
RUN make SHELL=/bin/ash tygo

# =============================================================================
# Stage 2: Build Frontend
# =============================================================================
FROM node:24.13.0-alpine AS frontend-builder

WORKDIR /app

# Install production dependencies only (build tools, not test/lint tools)
# Note: Extended timeout for arm64 builds which run through slow QEMU emulation
COPY package.json yarn.lock ./
RUN yarn config set network-timeout 600000 && \
    yarn install --production --frozen-lockfile

# Copy frontend source
COPY app/ ./app/
COPY index.html tsconfig.json tsconfig.app.json tsconfig.node.json vite.config.ts tailwind.config.js components.json ./

# Copy generated TypeScript types from typegen stage
COPY --from=typegen /app/app/types/generated/ ./app/types/generated/

# Build frontend
RUN NODE_ENV=production yarn build

# =============================================================================
# Stage 3: Build Backend
# =============================================================================
FROM golang:1.25.5-alpine AS backend-builder

ARG VERSION=dev

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

# Install dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY cmd/ ./cmd/
COPY pkg/ ./pkg/
COPY internal/ ./internal/

# Build static binary with version
RUN CGO_ENABLED=0 go build -o /app/shisho -installsuffix cgo \
    -ldflags "-w -s -X github.com/shishobooks/shisho/pkg/version.Version=${VERSION}" \
    ./cmd/api

# =============================================================================
# Stage 4: Final Production Image
# =============================================================================
FROM caddy:2-alpine

# Install su-exec for dropping privileges
RUN apk add --no-cache su-exec

# Create default non-root user
RUN addgroup -g 1000 shisho && \
    adduser -u 1000 -G shisho -s /bin/sh -D shisho

# Create necessary directories
RUN mkdir -p /config /srv && \
    chown -R shisho:shisho /config

WORKDIR /app

# Copy built artifacts
COPY --from=frontend-builder /app/build/app /srv
COPY --from=backend-builder /app/shisho /app/shisho

# Copy Caddyfile and entrypoint
COPY Caddyfile /etc/caddy/Caddyfile
COPY scripts/docker-entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Set environment variables
ENV LOG_FORMAT=json

# Default UID/GID (can be overridden with environment variables)
ENV PUID=1000
ENV PGID=1000

# Expose HTTP port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
