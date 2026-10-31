#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
BACKEND_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)

compose() {
	(
		CDPATH= cd -- "$BACKEND_DIR"
		docker compose "$@"
	)
}

if ! command -v docker >/dev/null 2>&1; then
	printf '%s\n' 'Docker is required for database checks.' >&2
	exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
	printf '%s\n' 'Docker Compose v2 is required for database checks.' >&2
	exit 1
fi

case "${1:-ready}" in
	config)
		compose config --quiet
		printf '%s\n' 'Database Compose configuration is valid.'
		;;
	ready)
		compose config --quiet
		running_services=$(compose ps --status running --services)
		if ! printf '%s\n' "$running_services" | grep -Fxq mongodb; then
			printf '%s\n' 'MongoDB is not running. Start it with `make db-up`, then retry.' >&2
			exit 1
		fi
		compose exec --no-TTY mongodb mongosh --quiet --eval 'if (!db.hello().isWritablePrimary) quit(1)'
		printf '%s\n' 'MongoDB replica-set primary is ready.'
		;;
	*)
		printf 'Usage: %s [config|ready]\n' "$0" >&2
		exit 2
		;;
esac