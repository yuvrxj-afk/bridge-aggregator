# ── Build stage ───────────────────────────────────────────────────────────────
FROM oven/bun:1.1.38-alpine AS builder

WORKDIR /app

# Copy workspace root manifests first — cached until they change.
COPY package.json bun.lock turbo.json ./
COPY apps/web/package.json ./apps/web/

RUN bun install --frozen-lockfile

# Copy all source (turbo needs the full workspace to resolve tasks).
COPY apps/web ./apps/web

# WalletConnect project ID is the only build-time env var needed.
# API URLs use relative paths proxied by nginx — no VITE_API_URL required.
# Network (mainnet/testnet) is a runtime toggle stored in localStorage.
ARG VITE_WALLETCONNECT_PROJECT_ID=
ENV VITE_WALLETCONNECT_PROJECT_ID=$VITE_WALLETCONNECT_PROJECT_ID

RUN bun run build:web

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine

COPY --from=builder /app/apps/web/dist /usr/share/nginx/html

# SPA routing + API proxy to backend services.
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
