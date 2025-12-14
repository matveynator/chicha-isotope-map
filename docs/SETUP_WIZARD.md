# Setup wizard (Linux)

This wizard walks through a short set of prompts and writes a port-specific `systemd` unit plus a matching log file path.

## Defaults you will see
- **Port**: prefilled from `-port` (8765 by default).
- **Domain**: empty keeps HTTP. Set a hostname for HTTPS via Let's Encrypt (needs 80/443 reachable).
- **Database**:
  - `pgx` = PostgreSQL. Defaults: host `localhost`, port `5432`, user `postgres`, empty password, DB `chicha`. The wizard builds the URI.
  - `sqlite` / `chai` / `duckdb` (when compiled in): suggests `/var/lib/<db-type>-<port>/database.<ext>` and creates missing directories.
  - `clickhouse`: prompts for a full URI.
- **Support e-mail**: shown in the legal notice.

## Editing while reviewing
At the review step:
- Press `1-4` to adjust a single field without retyping the rest.
- Press `Enter` to write the service.
- Type `restart` to rerun from the top with your current answers as defaults.
- Type `cancel` to abort and rerun later.

## Service and log names
The wizard bakes the port into everything so multiple instances can run side by side:
- Service: `chicha-isotope-map-<port>.service` under `/etc/systemd/system` (root) or `~/.config/systemd/user` (user).
- Logs: `/var/log/chicha-isotope-map-<port>.log` for system units, or `$XDG_STATE_HOME/chicha-isotope-map-<port>.log` for user units.

## Lifecycle commands
After writing the unit the wizard prints ready-to-use commands, mirroring `systemctl` defaults:
- `systemctl [--user] start chicha-isotope-map-<port>`
- `systemctl [--user] restart chicha-isotope-map-<port>`
- `systemctl [--user] stop chicha-isotope-map-<port>`
- `systemctl [--user] status chicha-isotope-map-<port>`
- `journalctl [--user] -u chicha-isotope-map-<port> -f` (or `tail -f` on the log file)
