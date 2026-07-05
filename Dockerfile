# syntax=docker/dockerfile:1
# Multi-stage build for PSR — keeps the final image small and secure.

# --- Build stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# Build a static-ish binary with trimpath and an embedded version.
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/psr ./cmd/psr

# --- Runtime stage ---
FROM alpine:3.20

# A non-root user improves the security posture.
RUN addgroup -S psr && adduser -S psr -G psr

# ca-certificates is required for HTTPS calls to hltv.org.
# tzdata keeps timestamps sane across zones.
RUN apk add --no-cache ca-certificates tzdata && \
    rm -rf /var/cache/apk/*

WORKDIR /app
COPY --from=builder /out/psr /app/psr

# Data directory for the SQLite DB, weights, and cookies.
RUN mkdir -p /app/data && chown -R psr:psr /app
USER psr

VOLUME ["/app/data"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health || exit 1

ENTRYPOINT ["/app/psr"]
CMD ["serve", "-addr", ":8080", "-db", "data/psr.db"]