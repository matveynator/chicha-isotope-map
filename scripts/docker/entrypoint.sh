#!/usr/bin/env bash
set -euo pipefail

# ----------- переменные по умолчанию -----------
PGDATA=${PGDATA:-/var/lib/postgresql/data}
DB_NAME=${DB_NAME:-chicha_isotope_map}
DB_USER=${DB_USER:-chicha_isotope_map}
PORT=${PORT:-8765}
CERT_DIR=/certs

BIN_PATH=/usr/local/bin/chicha-isotope-map           # бинарь внутри образа (fallback)
TMP_BIN=/tmp/chicha-isotope-map.new                  # куда качаем свежий
DOWNLOAD_URL="https://github.com/matveynator/chicha-isotope-map/releases/download/latest/chicha-isotope-map_linux_amd64"

umask 022
mkdir -p "$PGDATA" "$CERT_DIR"
# chown только если мы root (иначе это норм, просто пропускаем)
if [[ "$(id -u)" -eq 0 ]]; then
  chown -R postgres:postgres "$PGDATA" "$CERT_DIR"
fi

# ----------- попытка скачать свежий бинарь ------
RUN_BIN="$BIN_PATH"
echo ">>> checking for latest chicha-isotope-map at: $DOWNLOAD_URL"
if curl -fsSL --connect-timeout 10 --max-time 60 "$DOWNLOAD_URL" -o "$TMP_BIN"; then
  if [[ -s "$TMP_BIN" ]]; then
    # проверим, что это ELF (не html-страница ошибки)
    if [[ "$(head -c 4 "$TMP_BIN" 2>/dev/null || true)" == $'\177ELF' ]]; then
      chmod +x "$TMP_BIN"
      RUN_BIN="$TMP_BIN"
      echo ">>> will run freshly downloaded binary: $RUN_BIN"
    else
      echo ">>> downloaded file is not ELF, ignoring"
      rm -f "$TMP_BIN" || true
    fi
  else
    echo ">>> downloaded empty file, ignoring"
    rm -f "$TMP_BIN" || true
  fi
else
  echo ">>> unable to download latest binary, will use image binary"
fi

# ----------- initdb (один раз) -----------------
if [[ ! -s "$PGDATA/PG_VERSION" ]]; then
  gosu postgres initdb -D "$PGDATA" --auth=trust
  echo "listen_addresses = '127.0.0.1'"  >> "$PGDATA/postgresql.conf"
  echo "host all all 127.0.0.1/32 trust" >> "$PGDATA/pg_hba.conf"
fi

# ----------- запускаем PostgreSQL --------------
gosu postgres pg_ctl -D "$PGDATA" -w start
trap 'gosu postgres pg_ctl -D "$PGDATA" -m fast stop' EXIT

# ----------- роль + база (один раз) ------------
gosu postgres psql -v ON_ERROR_STOP=1 -qt <<SQL
DO \$\$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_USER}') THEN
    CREATE ROLE ${DB_USER} LOGIN;
  END IF;
END \$\$;
SQL

if ! gosu postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
  gosu postgres createdb -O "${DB_USER}" "${DB_NAME}"
fi

# ----------- аргументы приложения --------------
ARGS=(
  -db-type pgx
  -db-conn "postgres://${DB_USER}@127.0.0.1:5432/${DB_NAME}?sslmode=disable"
  -default-lat   "${DEFAULT_LAT:-51.389}"
  -default-lon   "${DEFAULT_LON:-30.099}"
  -default-zoom  "${DEFAULT_ZOOM:-4}"
  -default-layer "${DEFAULT_LAYER:-Google Satellite}"
  # Pass through the token so Mapbox tiles stay optional for Docker users.
  -mapbox-token  "${MAPBOX_TOKEN:-}"
)

# ----------- запуск приложения -----------------
# если указан DOMAIN → HTTPS (80/443) и оставляем root для bind низких портов
if [[ -n "${DOMAIN:-}" ]]; then
  ARGS+=( -domain "${DOMAIN}" )
  echo ">>> starting chicha-isotope-map (HTTPS) for domain ${DOMAIN} using: ${RUN_BIN}"
  exec "${RUN_BIN}" "${ARGS[@]}"
else
  ARGS+=( -port "${PORT}" )
  echo ">>> starting chicha-isotope-map on port ${PORT} using: ${RUN_BIN}"
  exec gosu postgres "${RUN_BIN}" "${ARGS[@]}"
fi
