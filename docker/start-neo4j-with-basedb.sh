#!/bin/bash
set -euo pipefail

BASE_DB="${KONDA_BASE_DB:-basedb}"
DUMP_DIR="${KONDA_DUMP_DIR:-/dump}"
AUTH_SPEC="${NEO4J_AUTH:?NEO4J_AUTH must be set}"
DB_PASSWORD="${AUTH_SPEC#neo4j/}"

if [ "${DB_PASSWORD}" = "${AUTH_SPEC}" ]; then
  echo "NEO4J_AUTH must be in the form neo4j/<password>" >&2
  exit 1
fi

case "$BASE_DB" in
  *[!a-zA-Z0-9_-]*)
    echo "Invalid BASE_DB value: $BASE_DB" >&2
    exit 1
    ;;
esac

if [ ! -d "/data/databases/${BASE_DB}" ]; then
  echo "Loading ${BASE_DB} from ${DUMP_DIR} ..."
  /var/lib/neo4j/bin/neo4j-admin database load "${BASE_DB}" --from-path="${DUMP_DIR}" --overwrite-destination=true
fi

/startup/docker-entrypoint.sh neo4j &
neo4j_pid=$!

cleanup() {
  if kill -0 "${neo4j_pid}" 2>/dev/null; then
    kill -TERM "${neo4j_pid}" 2>/dev/null || true
    wait "${neo4j_pid}" || true
  fi
}

trap cleanup INT TERM

until /var/lib/neo4j/bin/cypher-shell -a neo4j://localhost:7687 -u neo4j -p "${DB_PASSWORD}" -d system "SHOW DATABASES" >/dev/null 2>&1; do
  sleep 2
done

/var/lib/neo4j/bin/cypher-shell -a neo4j://localhost:7687 -u neo4j -p "${DB_PASSWORD}" -d system "CREATE DATABASE ${BASE_DB} IF NOT EXISTS"

wait "${neo4j_pid}"
