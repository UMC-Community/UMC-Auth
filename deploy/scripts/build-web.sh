#!/bin/sh
set -eu
cd "$(dirname "$0")/../web"

if ! command -v docker >/dev/null 2>&1; then
	echo "error: docker is required to build the web panel" >&2
	echo "Alternatively, with Node 22+ on the host: cd web && npm ci && npm run build" >&2
	exit 1
fi

if ! docker image inspect node:22-alpine >/dev/null 2>&1; then
	echo "Pulling node:22-alpine..."
	if ! docker pull node:22-alpine >/dev/null; then
		echo "error: could not pull node:22-alpine" >&2
		echo "With Node 22+ on the host, run: cd web && npm ci && npm run build" >&2
		exit 1
	fi
fi

echo "Building the account panel..."
docker run --rm \
	-u "$(id -u):$(id -g)" \
	-e HOME=/tmp \
	-e npm_config_cache=/tmp/.npm \
	-v "$PWD:/app" \
	-w /app \
	node:22-alpine sh -c 'npm ci && npm run build'

echo
echo "Built web/dist"
echo "Restart Nginx to serve the new files:"
echo "  cd .. && docker compose up -d nginx"
