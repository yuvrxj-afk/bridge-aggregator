# ── Build stage ───────────────────────────────────────────────────────────────
# Vite bakes VITE_* env vars into the bundle at build time.
# Pass them as --build-arg (e.g. --build-arg VITE_API_URL=https://api.example.com).
FROM oven/bun:1.1.38-alpine AS builder

WORKDIR /app

# Copy workspace root manifests first — cached until they change.
COPY package.json bun.lock turbo.json ./
COPY apps/web/package.json ./apps/web/

RUN bun install --frozen-lockfile

# Copy all source (turbo needs the full workspace to resolve tasks).
COPY apps/web ./apps/web

# Build args for Vite env vars — set real values via docker build --build-arg.
ARG VITE_API_URL=http://localhost:8080
ARG VITE_WALLETCONNECT_PROJECT_ID=
ARG VITE_NETWORK=mainnet

ENV VITE_API_URL=$VITE_API_URL
ENV VITE_WALLETCONNECT_PROJECT_ID=$VITE_WALLETCONNECT_PROJECT_ID
ENV VITE_NETWORK=$VITE_NETWORK

RUN bun run build:web

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM nginx:1.27-alpine

COPY --from=builder /app/apps/web/dist /usr/share/nginx/html

# SPA routing: all paths fall through to index.html.
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
