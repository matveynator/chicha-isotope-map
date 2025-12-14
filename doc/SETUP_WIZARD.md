# Guided setup wizard

Run `./chicha-isotope-map -setup` on Linux to open a coloured, interactive wizard that writes a systemd unit for you.

The wizard:
- asks for port, domain, database engine, database path/URI, and support e-mail;
- writes the unit to `/etc/systemd/system/chicha-isotope-map.service` when run as root, or to `~/.config/systemd/user/chicha-isotope-map.service` for user sessions;
- tries to run `systemctl daemon-reload`, `enable`, and `start` so the service comes up immediately;
- prints ready-to-use commands for starting, restarting, stopping, checking status, and following logs via `journalctl`.

If `systemctl` is unavailable or permissions are missing, the wizard still writes the service file and prints the exact commands to run manually.
