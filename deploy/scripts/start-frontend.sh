#!/bin/sh
set -e

# nginx handles /_next/static/* directly (gzip, immutable cache) and proxies
# everything else to Next.js on port 3001.
# Start nginx in the background; node becomes PID 1 so docker stop/SIGTERM
# reaches Next.js for a graceful shutdown.
nginx -c /etc/nginx/frontend.conf -g 'daemon off;' &

exec node /app/server.js
