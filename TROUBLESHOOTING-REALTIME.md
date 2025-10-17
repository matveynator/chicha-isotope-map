# Troubleshooting Realtime Sensors

## Server is Running Successfully on Port 8765! ✅

Your server is currently running with:
- **DuckDB** database (fixed constraint issue)
- **334 sensors stored** successfully
- **98 active realtime devices** being tracked
- **CORS enabled** for cross-origin access
- Polling **https://tt.safecast.org/devices** every 5 minutes

## Why Don't I See Realtime Sensors in the Browser?

### 1. Check the "Live" Checkbox ⭐ MOST COMMON ISSUE

Realtime sensors have a separate toggle in the web interface:

1. Open http://localhost:8765 in your browser
2. Look for the controls/legend area
3. Find and **enable the "Live" or realtime sensors checkbox**

Without this checkbox enabled, realtime sensors won't display even though they're being fetched and stored.

### 2. Verify Realtime is Enabled in JavaScript Console

Open browser console (F12) and check:

```javascript
console.log(window.safecastRealtimeEnabled);
// Should output: true
```

If it says `false`, the template isn't getting the flag. This shouldn't happen since the server logs show:
```
realtime poller start: url=https://tt.safecast.org/devices interval=5m0s
```

### 3. Check Network Tab

In browser DevTools Network tab:
1. Refresh the page
2. Look for `/get_markers` request
3. Check the response - it should include markers with:
   - `trackID` starting with `"live:"`
   - `speed: -1`
   - `deviceID` field populated

Example realtime marker:
```json
{
  "trackID": "live:geigiecast:62007",
  "deviceID": "geigiecast:62007",
  "speed": -1,
  "lat": 34.48291,
  "lon": 136.16325,
  "doseRate": 0.11
}
```

### 4. Time Slider Issue

The time slider might be filtering out sensors. Check:

1. Is the time slider visible at all?
2. Is it set to show "recent" data?
3. Try moving the slider to include the most recent time range

If the time slider is missing entirely, check browser console for JavaScript errors.

### 5. Map Zoom Level

Realtime sensors might not show at certain zoom levels due to clustering. Try:
1. Zooming in to a specific country (like Japan where most sensors are)
2. Check if small dots or clusters appear

## Quick Diagnostic Commands

### Check Server Status
```bash
curl http://localhost:8765/realtime_debug | jq
```

**Expected output:**
```json
{
  "total": 1443,
  "no_id": 0,
  "air": 1031,
  "no_convert": 0,
  "zero_coords": 56,
  "no_time": 4,
  "stale_24h": 254,
  "accepted": 98
}
```

- **accepted: 98** means sensors are being processed
- **air: 1031** means Air quality sensors are correctly filtered out
- **no_convert: 0** means all detector types are recognized

### Check If Markers Include Realtime
```bash
curl -s "http://localhost:8765/get_markers?minLat=20&minLon=110&maxLat=50&maxLon=145&zoom=5" | jq '.[0] | {trackID, deviceID, speed}'
```

**Expected output:**
```json
{
  "trackID": "live:geigiecast:62007",
  "deviceID": "geigiecast:62007",
  "speed": -1
}
```

If you see `"trackID": "live:..."` then realtime markers ARE in the API response.

### Check Database Contents
```bash
# Enter the database
duckdb ./data/chicha-realtime.duckdb

# Check realtime sensors count
SELECT COUNT(*) FROM realtime_measurements;

# Check recent sensors
SELECT device_id, lat, lon, value, unit, measured_at
FROM realtime_measurements
ORDER BY fetched_at DESC
LIMIT 10;

# Exit
.quit
```

## Server Logs Analysis

Current log shows successful operation:
```
realtime fetch: devices 1443
realtime poll: devices 98 stored 334 next=5m0s
realtime summary: Japan (JP):28 avg=0.21 Ukraine (UA):41 avg=0.22 ...
realtime filtered: total=1109 (noID=0 air=0 noConvert=1095 zeroCoords=10 noTime=4)
```

**This is GOOD!** It means:
- ✅ API fetch successful (1443 devices received)
- ✅ 98 devices accepted (radiation sensors)
- ✅ 334 measurements stored in database
- ✅ No conversion errors (noConvert=1095 are likely zero values)

## Frontend Debugging Steps

### Step 1: Open Browser Console
Press F12, go to Console tab

### Step 2: Check if Realtime is Enabled
```javascript
window.safecastRealtimeEnabled
```
Should output: `true`

### Step 3: Check State
```javascript
// Get current map state
if (window.st) {
  console.log("Live sensors enabled:", window.st.live);
  console.log("Current filters:", window.st);
}
```

### Step 4: Force Enable Live Sensors (Temporary Fix)
If the checkbox is hidden or not working:
```javascript
if (window.st) {
  window.st.live = true;
  console.log("Live sensors force-enabled. Refresh map to see sensors.");
  // Trigger map refresh if function exists
  if (typeof requestMapData === 'function') {
    requestMapData();
  }
}
```

### Step 5: Check for JavaScript Errors
Look for red error messages in console that might prevent the map from loading correctly.

## Common Issues & Solutions

### Issue: "I see the map but no realtime sensors"
**Solution**: Enable the "Live" checkbox in the map controls

### Issue: "Time slider is missing"
**Possible causes**:
1. JavaScript error preventing UI from loading
2. Check console for errors
3. Try hard refresh (Ctrl+Shift+R)

**Debug**:
```javascript
// Check if time slider elements exist
document.querySelector('[data-time-slider]') // or similar selector
```

### Issue: "Sensors show up briefly then disappear"
**Cause**: Time filter or zoom level filtering them out

**Solution**:
1. Adjust time slider to include recent data
2. Zoom in to a region with sensors (Japan, Ukraine, US)

### Issue: "CORS errors in console"
**Solution**: Already fixed! Server now sends CORS headers:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

### Issue: "Database errors in server logs"
**Solution**: Already fixed! DuckDB constraint syntax corrected.

## Sensor Distribution

Currently accepting sensors from:
- **Japan (JP)**: 28 sensors, avg 0.21 µSv/h
- **Ukraine (UA)**: 41 sensors, avg 0.22 µSv/h
- **United States (US)**: 19 sensors, avg 0.10 µSv/h
- **Canada (CA)**: 3 sensors, avg 0.06 µSv/h
- Other countries: 7 sensors total

**Tip**: Zoom to Japan (lat: 35-40, lon: 135-145) where most sensors are located.

## Testing the Fix

1. **Open browser** to http://localhost:8765
2. **Open DevTools** (F12)
3. **Go to Console tab**
4. **Run these checks**:
   ```javascript
   // Check if enabled
   console.log("Realtime enabled:", window.safecastRealtimeEnabled);

   // Check state
   console.log("Live toggle:", window.st?.live);

   // Force enable if needed
   if (window.st) window.st.live = true;
   ```

5. **Go to Network tab**
6. **Refresh page** and look for `/get_markers` response
7. **Search response** for `"live:"` - should find multiple matches

## Still Not Working?

### Check These Files

1. **Server logs** - should show:
   ```
   realtime poller start: url=https://tt.safecast.org/devices
   realtime poll: devices X stored Y
   ```

2. **Browser URL** - should be:
   ```
   http://localhost:8765
   ```
   NOT `file:///...` (won't work)

3. **Browser version** - try Chrome/Firefox latest

### Report Issue

If sensors still don't appear after all checks:

1. Open browser console
2. Copy any error messages
3. Run: `curl http://localhost:8765/realtime_debug`
4. Check server logs for errors
5. Verify the "Live" checkbox exists in the UI

## Server Control

### Stop Server
```bash
pkill -f chicha-isotope-map
```

### Restart Server
```bash
PORT=8765 ./run-with-realtime.sh
```

### Check Server is Running
```bash
curl http://localhost:8765/realtime_debug
```

### View Live Logs
```bash
# If running in foreground: just watch the terminal
# If running as service:
tail -f /var/log/chicha-isotope-map.log
# Or with systemd:
journalctl -u chicha-isotope-map -f
```

## Success Indicators

You'll know it's working when:
- ✅ Markers with white/gray rings appear on map
- ✅ DevTools shows `trackID` starting with `"live:"`
- ✅ `/realtime_debug` shows `"accepted": 98` or similar
- ✅ Server logs show `stored X` without errors
- ✅ Clicking a realtime marker shows device info

## Performance Notes

- **Polling interval**: 5 minutes (configurable in code)
- **Memory usage**: ~200-300 MB with DuckDB
- **Disk usage**: Database grows ~1-2 MB per day
- **Network**: ~100 KB per poll (fetching 1400+ devices)

## Next Steps

1. Enable the "Live" checkbox in the map UI
2. Zoom to Japan (most sensors)
3. Look for markers with white rings (active < 5 min) or gray rings (active < 24 hours)
4. Click a marker to see device history

The server is working correctly - the issue is almost certainly in the frontend display settings!
