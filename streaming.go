package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"chicha-isotope-map/pkg/database"
)

// aggregateMarkers chooses the most radioactive marker per grid cell.
// Cells shrink with higher zoom to preserve detail.
func aggregateMarkers(ctx context.Context, in <-chan database.Marker, zoom int) <-chan database.Marker {
	out := make(chan database.Marker)
	go func() {
		defer close(out)
		cells := make(map[string]database.Marker)
		scale := math.Pow(2, float64(zoom))
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-in:
				if !ok {
					return
				}
				key := fmt.Sprintf("%d:%d", int(m.Lat*scale), int(m.Lon*scale))
				if prev, ok := cells[key]; !ok || m.DoseRate > prev.DoseRate {
					cells[key] = m
					out <- m
				}
			}
		}
	}()
	return out
}

// streamMarkersHandler streams markers via Server-Sent Events.
// Markers are emitted as soon as they are read and aggregated.
func streamMarkersHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	zoom, _ := strconv.Atoi(q.Get("zoom"))
	minLat, _ := strconv.ParseFloat(q.Get("minLat"), 64)
	minLon, _ := strconv.ParseFloat(q.Get("minLon"), 64)
	maxLat, _ := strconv.ParseFloat(q.Get("maxLat"), 64)
	maxLon, _ := strconv.ParseFloat(q.Get("maxLon"), 64)

	ctx := r.Context()
	src, errCh := db.StreamMarkersByZoomAndBounds(ctx, zoom, minLat, minLon, maxLat, maxLon, *dbType)
	agg := aggregateMarkers(ctx, src, zoom)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				fmt.Fprintf(w, "event: done\ndata: %v\n\n", err)
			} else {
				fmt.Fprint(w, "event: done\ndata: end\n\n")
			}
			flusher.Flush()
			return
		case m, ok := <-agg:
			if !ok {
				fmt.Fprint(w, "event: done\ndata: end\n\n")
				flusher.Flush()
				return
			}
			b, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}
