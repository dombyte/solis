[![GitHub license](https://badgen.net/github/license/dombyte/solis)](https://github.com/dombyte/solis/blob/master/LICENSE)
[![Checks](https://github.com/dombyte/solis/actions/workflows/checks.yml/badge.svg)](https://github.com/dombyte/solis/actions/workflows/checks.yml)
[![GitHub go.mod Go version of a Go module](https://img.shields.io/github/go-mod/go-version/dombyte/solis.svg)](https://github.com/dombyte/solis)
[![Github tag](https://badgen.net/github/release/dombyte/solis/latest)](https://github.com/dombyte/solis/tags/)

# Solis Monitor

A self-hosted dashboard for Solis hybrid inverters. It reads the inverter over Modbus (TCP or
RS485), keeps your daily, monthly, yearly and total energy history, and shows live power flow
in the browser.

Tested with a **Solis S6 hybrid** inverter. Other Solis hybrid models with the same Modbus
register map may work too.

![Screenshot](.github/assets/screenshot.webp)

## Quick start (Docker)

```bash
cp example/docker-compose.yaml docker-compose.yaml   # or docker-compose.rtu.yaml for RS485
cp example/config.yaml config.yaml                   # set modbus.address
docker compose up -d
```

Open `http://<host>:8080`.

- Set `TZ` (e.g. `Europe/Berlin`) in the compose file; the timezone is not set in the config.
- Keep `restart: unless-stopped`: if the app cannot recover on its own, it exits and relies on
  Docker to start it again.

Without Docker, download a binary from the
[releases](https://github.com/dombyte/solis/releases) (Linux and macOS) and run `./solis`.

## Configuration

[`example/config.yaml`](example/config.yaml) is a commented template with every option. The
app reads `./config.yaml` (another file: `solis -config /path/to/config.yaml`). Any option can
also be set as an environment variable, e.g. `SOLIS_MODBUS_ADDRESS=tcp://192.168.1.200:502`.

The options you are most likely to change:

| Option | Default | Description |
|--------|---------|-------------|
| `modbus.address` | tcp://192.168.1.100:502 | `tcp://host:port` or `rtu://<device>` (e.g. `rtu:///dev/ttyUSB0`) |
| `modbus.slave_id` | 1 | Modbus unit ID of the inverter |
| `poller.interval` | 30s | How often the inverter is read |
| `app.port` | 8080 | Web port |
| `storage.path` | ./data/solis.db | Database file |
| `storage.daily_retention` | 10y | How long history is kept |

Durations need a unit (`30s`, `5m`, `24h`, `7d`, `1y`).

## Backups

The database is backed up daily to `backups/` next to it (the last 3 are kept). To restore,
stop the app, copy a backup over `solis.db` and delete `solis.db-wal` and `solis.db-shm`.

## Upgrading

Pull the new image (or binary) and restart; the database is migrated automatically after a
backup. Coming from **v1**: run v2.1.0 once first, then upgrade to v3.

If monthly or yearly values ever look wrong, stop the app and run
`solis backfill --years N` to recompute them from the daily history.

## Security

There is no login. Run it in your home network only, or behind a reverse proxy with
authentication if you need access from outside.

## Development

API docs (Swagger UI) are served at `/docs`. Build and contribution notes are in
[`AGENTS.md`](AGENTS.md).

## License

MIT — see [LICENSE](LICENSE).
