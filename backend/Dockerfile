# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Error-tracking release (e.g. v0.12); SENTRY_RELEASE at runtime overrides it.
ARG SENTRY_RELEASE=""
ARG RELEASE_LDFLAG="-X github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack.Release=${SENTRY_RELEASE}"

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w ${RELEASE_LDFLAG}" -o /out/server ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w ${RELEASE_LDFLAG}" -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/create-super-admin ./cmd/create-super-admin && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/roles-sync ./cmd/roles-sync && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/migrator ./cmd/migrator

FROM migrate/migrate:v4.19.1 AS migrate

FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 appuser

WORKDIR /app

COPY --from=migrate /usr/local/bin/migrate /usr/local/bin/migrate
COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /out/create-super-admin /app/create-super-admin
COPY --from=builder /out/roles-sync /app/roles-sync
COPY --from=builder /out/migrator /app/migrator
COPY migrations /app/migrations
COPY deploy/docker/entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh \
    && chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

ENV APP_HTTP_ADDR=:8080
ENV APP_ROLE=server
ENV AUTO_MIGRATE=true

ENTRYPOINT ["/app/entrypoint.sh"]
