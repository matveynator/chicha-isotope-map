package api

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
)

const (
	maxMercatorLatitude          = 85.05112878
	defaultOfflineBytesPerTile   = 28 * 1024
	maxOfflineEstimateRequestRaw = 16 * 1024
)

type offlineEstimateRequest struct {
	MinLat        float64  `json:"minLat"`
	MinLon        float64  `json:"minLon"`
	MaxLat        float64  `json:"maxLat"`
	MaxLon        float64  `json:"maxLon"`
	ZoomMin       int      `json:"zoomMin"`
	ZoomMax       int      `json:"zoomMax"`
	BaseLayers    []string `json:"baseLayers"`
	OverlayLayers []string `json:"overlayLayers"`
	BytesPerTile  int64    `json:"bytesPerTile"`
}

type offlineEstimateZoom struct {
	Zoom      int   `json:"zoom"`
	TileCount int64 `json:"tileCount"`
}

type offlineEstimateResponse struct {
	TileCount      int64                 `json:"tileCount"`
	LayerCount     int                   `json:"layerCount"`
	EstimatedBytes int64                 `json:"estimatedBytes"`
	EstimatedMiB   float64               `json:"estimatedMiB"`
	Breakdown      []offlineEstimateZoom `json:"breakdown"`
}

// handleOfflineEstimate returns a deterministic estimate used by the frontend
// before starting a large offline download job. We keep this endpoint
// stateless so callers can safely retry without side effects.
func (h *Handler) handleOfflineEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	permit, ok := h.acquirePermit(w, r, RequestGeneral)
	if !ok {
		return
	}
	if permit != nil {
		defer permit.Release()
	}

	requestPayload, err := decodeOfflineEstimateRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	responsePayload, err := buildOfflineEstimate(requestPayload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.respondJSON(w, responsePayload)
}

func decodeOfflineEstimateRequest(r *http.Request) (offlineEstimateRequest, error) {
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxOfflineEstimateRequestRaw))
	if err != nil {
		return offlineEstimateRequest{}, errors.New("read body")
	}

	var requestPayload offlineEstimateRequest
	if err := json.Unmarshal(bodyBytes, &requestPayload); err != nil {
		return offlineEstimateRequest{}, errors.New("invalid json")
	}

	return requestPayload, nil
}

func buildOfflineEstimate(requestPayload offlineEstimateRequest) (offlineEstimateResponse, error) {
	validatedRequest, err := normalizeOfflineEstimateRequest(requestPayload)
	if err != nil {
		return offlineEstimateResponse{}, err
	}

	totalTileCount := int64(0)
	breakdown := make([]offlineEstimateZoom, 0, validatedRequest.ZoomMax-validatedRequest.ZoomMin+1)

	for zoom := validatedRequest.ZoomMin; zoom <= validatedRequest.ZoomMax; zoom++ {
		tilesForZoom := estimateTileCountForZoom(validatedRequest.MinLat, validatedRequest.MinLon, validatedRequest.MaxLat, validatedRequest.MaxLon, zoom)
		totalTileCount += tilesForZoom
		breakdown = append(breakdown, offlineEstimateZoom{
			Zoom:      zoom,
			TileCount: tilesForZoom,
		})
	}

	layerCount := countUniqueLayerNames(validatedRequest.BaseLayers, validatedRequest.OverlayLayers)
	estimatedBytes := totalTileCount * int64(layerCount) * validatedRequest.BytesPerTile
	estimatedMiB := float64(estimatedBytes) / (1024.0 * 1024.0)

	return offlineEstimateResponse{
		TileCount:      totalTileCount,
		LayerCount:     layerCount,
		EstimatedBytes: estimatedBytes,
		EstimatedMiB:   math.Round(estimatedMiB*100) / 100,
		Breakdown:      breakdown,
	}, nil
}

func normalizeOfflineEstimateRequest(requestPayload offlineEstimateRequest) (offlineEstimateRequest, error) {
	requestPayload.MinLat = clampFloat(requestPayload.MinLat, -maxMercatorLatitude, maxMercatorLatitude)
	requestPayload.MaxLat = clampFloat(requestPayload.MaxLat, -maxMercatorLatitude, maxMercatorLatitude)

	if requestPayload.MinLat > requestPayload.MaxLat {
		requestPayload.MinLat, requestPayload.MaxLat = requestPayload.MaxLat, requestPayload.MinLat
	}

	if requestPayload.MinLon < -180 || requestPayload.MinLon > 180 || requestPayload.MaxLon < -180 || requestPayload.MaxLon > 180 {
		return offlineEstimateRequest{}, errors.New("lon out of range")
	}

	if requestPayload.ZoomMin < 0 || requestPayload.ZoomMin > 22 {
		return offlineEstimateRequest{}, errors.New("zoomMin out of range")
	}
	if requestPayload.ZoomMax < 0 || requestPayload.ZoomMax > 22 {
		return offlineEstimateRequest{}, errors.New("zoomMax out of range")
	}
	if requestPayload.ZoomMin > requestPayload.ZoomMax {
		return offlineEstimateRequest{}, errors.New("zoomMin must be <= zoomMax")
	}

	if requestPayload.BytesPerTile <= 0 {
		requestPayload.BytesPerTile = defaultOfflineBytesPerTile
	}

	return requestPayload, nil
}

func countUniqueLayerNames(baseLayers []string, overlayLayers []string) int {
	uniqueNames := map[string]struct{}{}
	for _, rawLayerName := range baseLayers {
		layerName := strings.TrimSpace(rawLayerName)
		if layerName == "" {
			continue
		}
		uniqueNames[layerName] = struct{}{}
	}
	for _, rawLayerName := range overlayLayers {
		layerName := strings.TrimSpace(rawLayerName)
		if layerName == "" {
			continue
		}
		uniqueNames[layerName] = struct{}{}
	}

	if len(uniqueNames) == 0 {
		return 1
	}
	return len(uniqueNames)
}

func estimateTileCountForZoom(minLat float64, minLon float64, maxLat float64, maxLon float64, zoom int) int64 {
	scale := int64(1) << uint(zoom)
	minY := latToTileY(maxLat, zoom)
	maxY := latToTileY(minLat, zoom)

	verticalTileCount := int64(maxY - minY + 1)
	if verticalTileCount <= 0 {
		return 0
	}

	minX := lonToTileX(minLon, zoom)
	maxX := lonToTileX(maxLon, zoom)
	if minLon <= maxLon {
		horizontalTileCount := int64(maxX - minX + 1)
		if horizontalTileCount <= 0 {
			return 0
		}
		return horizontalTileCount * verticalTileCount
	}

	leftSegmentTileCount := scale - int64(minX)
	rightSegmentTileCount := int64(maxX + 1)
	return (leftSegmentTileCount + rightSegmentTileCount) * verticalTileCount
}

func lonToTileX(lon float64, zoom int) int {
	scale := float64(int64(1) << uint(zoom))
	normalized := (lon + 180.0) / 360.0
	tile := int(math.Floor(normalized * scale))
	if tile < 0 {
		return 0
	}
	maxTile := (1 << zoom) - 1
	if tile > maxTile {
		return maxTile
	}
	return tile
}

func latToTileY(lat float64, zoom int) int {
	clampedLat := clampFloat(lat, -maxMercatorLatitude, maxMercatorLatitude)
	latRad := clampedLat * math.Pi / 180.0
	scale := float64(int64(1) << uint(zoom))
	projected := (1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2
	tile := int(math.Floor(projected * scale))
	if tile < 0 {
		return 0
	}
	maxTile := (1 << zoom) - 1
	if tile > maxTile {
		return maxTile
	}
	return tile
}
