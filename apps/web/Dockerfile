# ── Build stage ─────────────────────────────────────────────
FROM oven/bun:1.1.38-alpine AS builder

WORKDIR /app

RUN apk add --no-cache python3 make g++ linux-headers eudev-dev pkgconfig

COPY package.json bun.lock turbo.json ./
COPY apps/web/package.json ./apps/web/

# ✅ FIXED INSTALL
RUN bun install --frozen-lockfile --omit=optional --allow-scripts

COPY apps/web ./apps/web

ARG VITE_WALLETCONNECT_PROJECT_ID=
ENV VITE_WALLETCONNECT_PROJECT_ID=$VITE_WALLETCONNECT_PROJECT_ID

# Build bundle directly and assert output folder exists.
# Keep type-checking in separate CI/lint step to avoid deployment blocking.
RUN cd apps/web \
 && bunx vite build \
 && test -d dist

# ── Runtime ─────────────────────────────────────────────
FROM nginx:1.27-alpine

RUN apk add --no-cache gettext

COPY --from=builder /app/apps/web/dist /usr/share/nginx/html
COPY apps/web/nginx.conf /etc/nginx/templates/default.conf.template

ENV API_UPSTREAM=api:8080
ENV API_TESTNET_UPSTREAM=api-testnet:8080

EXPOSE 80

CMD ["/bin/sh", "-c", "envsubst '${API_UPSTREAM} ${API_TESTNET_UPSTREAM}' < /etc/nginx/templates/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]