# Realtime Sensors Setup Guide

This guide explains how to enable and use the Safecast realtime sensors feature.

## Quick Start

### Option 1: Use the convenience script (recommended)

```bash
./run-with-realtime.sh
```

This will:
- Build the application with DuckDB support
- Start the server on port 8080 (customizable)
- Enable realtime sensor polling every 5 minutes
- Create database at `./data/chicha-realtime.duckdb`

### Option 2: Build and run manually

```bash
# Build with DuckDB support
./build-duckdb.sh

# Run with realtime enabled
./chicha-isotope-map \
    -db-type=duckdb \
    -db-path=./data/chicha-realtime.duckdb \
    -port=8080 \
    -safecast-realtime=true
```

## Configuration Options

### Database Types

The application supports multiple database backends:

- **DuckDB** (recommended for new installations)
  ```bash
  -db-type=duckdb -db-path=./data/db.duckdb
  ```
  - Requires CGO and C compiler
  - Excellent performance
  - Single-file database

- **SQLite** (default, no CGO required)
  ```bash
  -db-type=sqlite -db-path=./data/db.sqlite
  ```

- **PostgreSQL**
  ```bash
  -db-type=pgx -db-conn="host=localhost port=5432 user=postgres dbname=IsotopePathways"
  ```

- **ClickHouse**
  ```bash
  -db-type=clickhouse -db-conn="host=localhost port=9000 database=IsotopePathways"
  ```

### Environment Variables

Customize the run script with environment variables:

```bash
# Change database location
DB_PATH=./my-custom-db.duckdb ./run-with-realtime.sh

# Change port
PORT=3000 ./run-with-realtime.sh

# Both
DB_PATH=/var/lib/chicha/db.duckdb PORT=3000 ./run-with-realtime.sh
```

## What's New

### 1. Additional Detector Support

The following detector types are now supported:

- **LND 7317/7318** - 334 CPM per µSv/h
- **LND 712/7128** - 108 CPM per µSv/h
- **LND 78017** - 57 CPM per µSv/h (NEW!)
- **SBM-20** - 175 CPM per µSv/h (NEW!)
- **J305** - 153 CPM per µSv/h (NEW!)

### 2. Enhanced Logging

The server now logs detailed filtering statistics:

```
realtime fetch: devices 150
realtime summary: Japan (JP):45 avg=0.08 USA (US):32 avg=0.12 added=3 removed=1
realtime filtered: total=42 (noID=5 air=12 noConvert=8 zeroCoords=3 noTime=14)
realtime unconverted: device=geigiecast:12345 value=38.00 unit="lnd_custom"
```

### 3. Debug Endpoint

Access real-time diagnostics at:

```
http://localhost:8080/realtime_debug
```

Example response:
```json
{
  "total": 150,
  "no_id": 0,
  "air": 25,
  "no_convert": 12,
  "zero_coords": 3,
  "no_time": 5,
  "stale_24h": 18,
  "accepted": 87,
  "unconverted_units": ["lnd_custom", "lnd_unknown"],
  "unconverted_device_ids": ["geigiecast:12345", "geigiecast:67890"]
}
```

## How It Works

### Data Flow

1. **Polling**: Server polls `https://tt.safecast.org/devices` every 5 minutes
2. **Filtering**: Applies quality filters:
   - Removes devices without IDs
   - Filters out Safecast Air devices (not radiation sensors)
   - Skips sensors with zero coordinates
   - Removes readings older than 24 hours
   - Converts known detector types to µSv/h
3. **Storage**: Stores accepted readings in database
4. **Display**: Shows live markers on the map with:
   - White ring = active (< 5 minutes old)
   - Gray ring = recent (5 min - 24 hours old)

### Filtering Rules

Sensors are **filtered out** if:
- Missing device ID
- Labeled as "Air" device (air quality, not radiation)
- Both lat and lon are 0.0
- Missing timestamp
- Last reading is >24 hours old
- Unknown detector type (cannot convert to µSv/h)
- Non-positive radiation value

## Troubleshooting

### No sensors appearing?

1. **Check if feature is enabled**:
   ```bash
   # Look for this in logs:
   realtime poller start: url=https://tt.safecast.org/devices interval=5m0s
   ```

2. **Check the debug endpoint**:
   ```bash
   curl http://localhost:8080/realtime_debug | jq
   ```

3. **Check logs for filtering reasons**:
   ```bash
   # Look for lines like:
   realtime filtered: total=42 (noID=5 air=12 noConvert=8 ...)
   ```

4. **Check frontend toggle**:
   - Open the map in browser
   - Ensure the "live" sensors checkbox is enabled

### Build fails with "C compiler required"?

DuckDB requires CGO. Install build tools:

**Ubuntu/Debian**:
```bash
sudo apt-get install build-essential
```

**macOS**:
```bash
xcode-select --install
```

**Windows**:
- Install MinGW-w64 or TDM-GCC
- Or use SQLite instead: `-db-type=sqlite`

### Port already in use?

Change the port:
```bash
PORT=3000 ./run-with-realtime.sh
```

Or manually:
```bash
./chicha-isotope-map -port=3000 -safecast-realtime=true -db-type=duckdb
```

## Performance

### Database Comparison

| Database   | Build Time | Query Speed | File Size | CGO Required |
|------------|------------|-------------|-----------|--------------|
| DuckDB     | Slower     | Excellent   | Small     | Yes          |
| SQLite     | Fast       | Good        | Small     | No           |
| PostgreSQL | N/A        | Excellent   | N/A       | No           |
| ClickHouse | N/A        | Excellent   | N/A       | No           |

### Recommendations

- **Development**: SQLite (no CGO, fast builds)
- **Production**: DuckDB or PostgreSQL (best performance)
- **Large Scale**: ClickHouse (billions of records)

## API Endpoints

| Endpoint              | Description                                    |
|-----------------------|------------------------------------------------|
| `/`                   | Main map interface                             |
| `/get_markers`        | Get all markers (includes realtime sensors)    |
| `/realtime_history`   | Get 1-year history for a specific device       |
| `/realtime_debug`     | Diagnostic info about sensor filtering (NEW!)  |
| `/api/docs`           | API documentation                              |

## Advanced Usage

### Running as a Service

Create `/etc/systemd/system/chicha-isotope-map.service`:

```ini
[Unit]
Description=Chicha Isotope Map with Realtime Sensors
After=network.target

[Service]
Type=simple
User=chicha
WorkingDirectory=/opt/chicha-isotope-map
ExecStart=/opt/chicha-isotope-map/chicha-isotope-map \
    -db-type=duckdb \
    -db-path=/var/lib/chicha/db.duckdb \
    -safecast-realtime=true \
    -port=8080
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable chicha-isotope-map
sudo systemctl start chicha-isotope-map
sudo systemctl status chicha-isotope-map
```

View logs:
```bash
sudo journalctl -u chicha-isotope-map -f
```

## Support

For issues or questions:
- Check logs for filtering statistics
- Use `/realtime_debug` endpoint for diagnostics
- Review this document's troubleshooting section
- Check the main README.md for general issues
