# ── Build stage ───────────────────────────────────────────────────────────────
FROM oven/bun:1.1.38-alpine AS builder

WORKDIR /app

# Native build deps required by transitive node-gyp packages (e.g. usb).
RUN apk add --no-cache python3 make g++ linux-headers eudev-dev pkgconfig

# Copy workspace root manifests first — cached until they change.
COPY package.json bun.lock turbo.json ./
COPY apps/web/package.json ./apps/web/

RUN bun install --frozen-lockfile --filter=@apps/web --omit=optional

# Copy all source (turbo needs the full workspace to resolve tasks).
COPY apps/web ./apps/web

# WalletConnect project ID is the only build-time env var needed.
# API URLs use relative paths proxied by nginx — no VITE_API_URL required.
# Network (mainnet/testnet) is a runtime toggle stored in localStorage.
ARG VITE_WALLETCONNECT_PROJECT_ID=
ENV VITE_WALLETCONNECT_PROJECT_ID=$VITE_WALLETCONNECT_PROJECT_ID

# Run build tools directly to avoid workspace script resolution quirks in CI.
RUN bun --cwd apps/web x tsc -b && bun --cwd apps/web x vite build

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine

# Install gettext for envsubst (needed for nginx config templating).
RUN apk add --no-cache gettext

COPY --from=builder /app/apps/web/dist /usr/share/nginx/html

# nginx.conf uses ${API_UPSTREAM} and ${API_TESTNET_UPSTREAM} placeholders.
# Defaults match docker-compose service names; override for other environments.
COPY apps/web/nginx.conf /etc/nginx/templates/default.conf.template
ENV API_UPSTREAM=api:8080
ENV API_TESTNET_UPSTREAM=api-testnet:8080

EXPOSE 80

# envsubst substitutes only the two upstream vars — nginx's own $uri, $host, etc. are preserved.
CMD ["/bin/sh", "-c", "envsubst '${API_UPSTREAM} ${API_TESTNET_UPSTREAM}' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]
