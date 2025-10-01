// Paste this into your browser console (F12) to debug real-time sensors

console.log("=== Real-time Sensor Debug ===");

// 1. Check if backend has real-time enabled
console.log("Backend real-time enabled:", window.safecastRealtimeEnabled);

// 2. Check speed filter state
const filterState = loadSpeedFilterState();
console.log("Speed filter state:", filterState);
console.log("Live sensors allowed:", filterState.live);

// 3. Check if there are any markers on the map
console.log("Total circle markers:", Object.keys(circleMarkers).length);

// 4. Check marker stacks
console.log("Marker stacks:", markerStacks.size);

// 5. List all markers and identify real-time ones
let realtimeCount = 0;
let historicalCount = 0;
for (const key in circleMarkers) {
  const marker = circleMarkers[key];
  if (marker && marker.options && marker.options.markerData) {
    const data = marker.options.markerData;
    if (isRealtimeMarker(data)) {
      realtimeCount++;
      console.log("Real-time marker:", {
        id: data.deviceID || data.device_id,
        trackID: data.trackID,
        lat: data.lat,
        lon: data.lon,
        dose: data.doseRate,
        speed: data.speed
      });
    } else {
      historicalCount++;
    }
  }
}

console.log("Real-time markers visible:", realtimeCount);
console.log("Historical markers visible:", historicalCount);

// 6. Check current map bounds
const bounds = map.getBounds();
console.log("Current map bounds:", {
  south: bounds.getSouth(),
  west: bounds.getWest(),
  north: bounds.getNorth(),
  east: bounds.getEast()
});

// 7. Check if we're in track view mode
console.log("Track view mode:", isTrackView);
console.log("Current track ID:", currentTrackID);

console.log("=== End Debug ===");
