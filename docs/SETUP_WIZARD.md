# Guided setup wizard

Run `./chicha-isotope-map -setup` on Linux to open a coloured, interactive wizard that writes a systemd unit for you.

The wizard:
- asks for port, domain, database engine, database path/URI, and support e-mail;
- writes the unit as `chicha-isotope-map-<port>.service` so multiple ports can coexist (root installs to `/etc/systemd/system`, user sessions to `~/.config/systemd/user`);
- streams stdout and stderr into `chicha-isotope-map-<port>.log` alongside `journalctl` so you can tail plain files when preferred;
- tries to run `systemctl daemon-reload`, `enable`, and `start` so the service comes up immediately;
- prints ready-to-use commands for starting, restarting, stopping, checking status, and following logs via `journalctl` or by tailing the log file.

If `systemctl` is unavailable or permissions are missing, the wizard still writes the service file and prints the exact commands to run manually.
