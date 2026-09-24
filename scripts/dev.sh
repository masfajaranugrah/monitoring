#!/usr/bin/env bash
# ============================================================
#  Monitoring — Development runner (macOS/Homebrew)
#  Menjalankan PostgreSQL + PostGIS lokal, build backend, run.
#
#  Penggunaan:
#    ./scripts/dev.sh start     # jalankan database + backend
#    ./scripts/dev.sh stop      # hentikan semua
#    ./scripts/dev.sh status    # cek status
#    ./scripts/dev.sh logs      # tail log backend
# ============================================================
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DATA_DIR="$ROOT/.data"
PGDATA="$DATA_DIR/pg"
PGPORT="${PGPORT:-55432}"
PHPOSTGIS_BIN=""
LOG_DIR="$DATA_DIR/logs"

# deteksi binary postgres (Homebrew @17 atau @16)
if [[ -x /opt/homebrew/opt/postgresql@17/bin/postgres ]]; then
  PGBIN=/opt/homebrew/opt/postgresql@17/bin
elif [[ -x /opt/homebrew/opt/postgresql@16/bin/postgres ]]; then
  PGBIN=/opt/homebrew/opt/postgresql@16/bin
else
  echo "ERROR: PostgreSQL (homebrew) tidak ditemukan. Install: brew install postgresql@17 postgis"
  exit 1
fi

mkdir -p "$LOG_DIR"
export PATH="$PGBIN:$PATH"

db_is_running() {
  [[ -f "$PGDATA/postmaster.pid" ]] || return 1
  "$PGBIN/pg_ctl" -D "$PGDATA" status >/dev/null 2>&1
}

ensure_db() {
  if [[ ! -d "$PGDATA/base" ]]; then
    echo "==> Init PostgreSQL data dir: $PGDATA"
    initdb -D "$PGDATA" -U monitor -A trust --no-locale -E UTF8 >/dev/null
  fi
  if ! db_is_running; then
    echo "==> Start PostgreSQL on port $PGPORT"
    "$PGBIN/pg_ctl" -D "$PGDATA" \
      -o "-p $PGPORT -k $PGDATA -c listen_addresses=127.0.0.1" \
      -l "$LOG_DIR/postgres.log" start >/dev/null
    sleep 2
  fi

  # Buat database + postgis bila belum ada
  if ! "$PGBIN/psql" -h "$PGDATA" -p "$PGPORT" -U monitor -d postgres -tAc \
       "SELECT 1 FROM pg_database WHERE datname='monitoring'" | grep -q 1; then
    echo "==> Create database monitoring"
    "$PGBIN/createdb" -h "$PGDATA" -p "$PGPORT" -U monitor monitoring
    "$PGBIN/psql" -h "$PGDATA" -p "$PGPORT" -U monitor -d monitoring \
      -c "CREATE EXTENSION IF NOT EXISTS postgis;" >/dev/null
  fi
}

backend_is_running() {
  [[ -f "$DATA_DIR/backend.pid" ]] && kill -0 "$(cat "$DATA_DIR/backend.pid")" 2>/dev/null
}

start_backend() {
  if backend_is_running; then
    echo "==> Backend sudah berjalan (pid $(cat "$DATA_DIR/backend.pid"))"
    return
  fi
  echo "==> Build backend..."
  (cd "$ROOT/backend" && go build -o "$ROOT/.data/monitoring-server" ./cmd/server)

  # .env lokal bila belum ada
  [[ -f "$ROOT/.env" ]] || { echo "==> Salin .env.example -> .env"; cp "$ROOT/.env.example" "$ROOT/.env"; }

  # Pastikan DATABASE_URL sesuai postgres lokal (port $PGPORT)
  export DATABASE_URL="postgres://monitor@127.0.0.1:$PGPORT/monitoring?sslmode=disable"
  export SERVER_PORT="${SERVER_PORT:-8080}"
  export GIN_MODE="${GIN_MODE:-release}"
  export JWT_SECRET="${JWT_SECRET:-local-dev-secret-change-me-0123456789abcdef}"
  export ENCRYPTION_KEY="${ENCRYPTION_KEY:-local-dev-encryption-key}"
  export PING_CONCURRENCY="${PING_CONCURRENCY:-10}"
  export ADMIN_INITIAL_USERNAME="${ADMIN_INITIAL_USERNAME:-admin}"
  export ADMIN_INITIAL_PASSWORD="${ADMIN_INITIAL_PASSWORD:-admin123}"

  echo "==> Start backend (http://localhost:$SERVER_PORT)"
  nohup "$ROOT/.data/monitoring-server" > "$LOG_DIR/backend.log" 2>&1 &
  echo $! > "$DATA_DIR/backend.pid"
  sleep 2
  echo "==> Backend log:"
  tail -5 "$LOG_DIR/backend.log"
}

case "${1:-start}" in
  start)
    ensure_db
    start_backend
    echo ""
    echo "▓▓ Monitoring ▓▓"
    echo "  Backend : http://localhost:${SERVER_PORT:-8080}  (API + SPA)"
    echo "  Login   : admin / admin123  (ubah setelah login)"
    echo ""
    echo "Frontend dev (bagian kedua, jalankan di terminal lain):"
    echo "  cd frontend && npm run dev   ->  http://localhost:5173"
    ;;
  stop)
    if backend_is_running; then
      kill "$(cat "$DATA_DIR/backend.pid")" 2>/dev/null || true
      rm -f "$DATA_DIR/backend.pid"
      echo "==> Backend berhenti"
    fi
    if db_is_running; then
      "$PGBIN/pg_ctl" -D "$PGDATA" stop >/dev/null 2>&1
      echo "==> PostgreSQL berhenti"
    fi
    ;;
  status)
    echo -n "PostgreSQL: "; db_is_running && echo "running" || echo "stopped"
    echo -n "Backend  : "; backend_is_running && echo "running" || echo "stopped"
    ;;
  logs)
    tail -f "$LOG_DIR/backend.log"
    ;;
  *)
    echo "Usage: $0 {start|stop|status|logs}"
    exit 1
    ;;
esac