# syntax=docker/dockerfile:1.7

# ---- Build stage ----
FROM node:20-alpine AS builder

WORKDIR /src

# Install deps dengan cache yang efisien (copy package files dulu)
COPY package.json package-lock.json* ./
RUN npm ci --no-audit --no-fund

# Copy source dan build
COPY . .

# VITE_API_TARGET tidak relevan untuk production build (proxy dilakukan nginx),
# tapi di-set agar build script tidak error kalau ada referensi
ENV NODE_ENV=production
RUN npm run build

# ---- Runtime stage ----
FROM nginx:1.27-alpine

# Hapus default config
RUN rm /etc/nginx/conf.d/default.conf

# Copy build output dan custom config
COPY --from=builder /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Non-root: nginx:alpine sudah punya user nginx (uid 101). 
# Master process tetap root untuk bind port 80, worker akan drop ke nginx user.
# Untuk truly non-root, butuh setup tambahan; default sudah cukup aman untuk
# container behind reverse proxy (cloudflared).

EXPOSE 80

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1/health.txt >/dev/null 2>&1 || exit 1

# Tulis health probe file di build time
RUN echo "ok" > /usr/share/nginx/html/health.txt

CMD ["nginx", "-g", "daemon off;"]
