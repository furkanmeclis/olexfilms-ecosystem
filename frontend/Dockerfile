# Build gate: empty by default. compose.prod.yml replaces it with the backend
# image (additional_contexts), so the frontend build starts only after the
# backend image is built. Keeps the two heavy builds from running together.
FROM scratch AS build-gate

FROM node:22.22.0-alpine3.23 AS base

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH

# Pin pnpm — pnpm@latest (12.x) currently breaks under corepack
# (MODULE_NOT_FOUND …/pnpm.cjs). Match local lockfile toolchain.
RUN corepack enable && corepack prepare pnpm@11.22.0 --activate

WORKDIR /app

FROM base AS deps

# Build context is the project root; frontend sources are under frontend/
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN --mount=from=build-gate,target=/tmp/build-gate \
    pnpm install --frozen-lockfile

FROM base AS build

ENV NEXT_TELEMETRY_DISABLED=1
# Public site origin for canonical / Open Graph / sitemap URLs. Static pages
# (landing, OG image, robots, sitemap) bake it in at build time.
ARG SITE_URL=""
ENV NEXT_PUBLIC_SITE_URL=$SITE_URL
# Browser error tracking (TEC-82): empty DSN = no client SDK. Not secrets.
ARG NEXT_PUBLIC_SENTRY_DSN=""
ARG NEXT_PUBLIC_SENTRY_ENVIRONMENT=""
ARG NEXT_PUBLIC_SENTRY_RELEASE=""
ENV NEXT_PUBLIC_SENTRY_DSN=$NEXT_PUBLIC_SENTRY_DSN \
    NEXT_PUBLIC_SENTRY_ENVIRONMENT=$NEXT_PUBLIC_SENTRY_ENVIRONMENT \
    NEXT_PUBLIC_SENTRY_RELEASE=$NEXT_PUBLIC_SENTRY_RELEASE

COPY --from=deps /app/node_modules ./node_modules
COPY frontend/ .

RUN pnpm build

FROM node:22.22.0-alpine3.23 AS runtime

ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV HOSTNAME=0.0.0.0
# nginx listens on 3000 (public); Next.js listens on 3001 (internal only)
ENV PORT=3001

# Install nginx to serve /_next/static/* directly (fast static delivery + gzip)
RUN apk add --no-cache nginx

RUN addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app \
    && chown -R app:app /var/lib/nginx /var/log/nginx 2>/dev/null || true

WORKDIR /app

COPY --from=build --chown=app:app /app/public ./public
COPY --from=build --chown=app:app /app/.next/standalone ./
COPY --from=build --chown=app:app /app/.next/static ./.next/static

COPY --chown=app:app deploy/nginx/frontend.conf /etc/nginx/frontend.conf
COPY --chown=app:app deploy/scripts/start-frontend.sh /app/start.sh
RUN chmod +x /app/start.sh

USER app

EXPOSE 3000

HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=10 \
  CMD ["node", "-e", "fetch('http://127.0.0.1:3000/health').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"]

CMD ["/app/start.sh"]
