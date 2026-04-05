# =============================================================================
# Stage 1: Generate TypeScript Types
# =============================================================================
FROM golang:1.25.5-alpine AS typegen

WORKDIR /app

# Install tygo (pinned to same version as .mise.toml)
RUN go install github.com/gzuidhof/tygo@v0.2.20

# Copy Go modules for type generation
COPY go.mod go.sum ./
RUN go mod download

# Copy source files and config needed for type generation
COPY tygo.yaml ./
COPY pkg/ ./pkg/

# Generate TypeScript types
RUN tygo generate

# =============================================================================
# Stage 2: Build Frontend
# =============================================================================
FROM node:25.9.0-alpine AS frontend-builder

WORKDIR /app

# Install pnpm via corepack (reads version from packageManager field in package.json)
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY packages/plugin-sdk/package.json ./packages/plugin-sdk/
RUN corepack enable && corepack install

# Install production dependencies only (build tools, not test/lint tools)
# Note: fetch-timeout for arm64/QEMU is configured in .npmrc
RUN pnpm install --prod --frozen-lockfile

# Copy frontend source
COPY app/ ./app/
COPY public/ ./public/
COPY index.html tsconfig.json tsconfig.app.json tsconfig.node.json vite.config.ts tailwind.config.js components.json ./

# Copy generated TypeScript types from typegen stage
COPY --from=typegen /app/app/types/generated/ ./app/types/generated/

# Build frontend
RUN NODE_ENV=production pnpm build

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

# Startup timeout for backend health check (increase for slow storage like NAS)
ENV STARTUP_TIMEOUT_SECONDS=120

# Expose HTTP port
EXPOSE 5173

# Health check (start-period allows time for initial migrations on slow storage)
HEALTHCHECK --interval=30s --timeout=3s --start-period=120s --retries=3 \
    CMD wget -q --spider http://localhost:5173/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
