# ── Build stage ───────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# ca-certificates needed for TLS to external APIs (Across, Circle, etc.)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Download deps before copying source — layer is cached on go.mod/go.sum changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a static binary; strip debug info to keep image small.
# GOARCH is intentionally omitted — Docker's --platform flag or BuildKit handles it,
# so the same Dockerfile works on amd64 (CI) and arm64 (Apple Silicon, Graviton).
RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -o server ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12

WORKDIR /app

COPY --from=builder /app/server .
# Config YAML (contains defaults; all values are overridable via env vars).
COPY --from=builder /app/internal/config/config.yaml ./internal/config/config.yaml

EXPOSE 8080

# Distroless does not have a shell — use exec form.
ENTRYPOINT ["/app/server"]
