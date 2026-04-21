package api

import "testing"

func TestEstimateTileCountForZoom_WorldZoom0(t *testing.T) {
	gotTileCount := estimateTileCountForZoom(-85, -180, 85, 180, 0)
	if gotTileCount != 1 {
		t.Fatalf("expected 1 tile at z0, got %d", gotTileCount)
	}
}

func TestEstimateTileCountForZoom_WorldZoom1(t *testing.T) {
	gotTileCount := estimateTileCountForZoom(-85, -180, 85, 180, 1)
	if gotTileCount != 4 {
		t.Fatalf("expected 4 tiles at z1, got %d", gotTileCount)
	}
}

func TestEstimateTileCountForZoom_CrossesAntimeridian(t *testing.T) {
	gotTileCount := estimateTileCountForZoom(-10, 170, 10, -170, 3)
	if gotTileCount <= 0 {
		t.Fatalf("expected positive tile count, got %d", gotTileCount)
	}
}

func TestBuildOfflineEstimate_DefaultLayerAndBytesFallback(t *testing.T) {
	requestPayload := offlineEstimateRequest{
		MinLat:  10,
		MinLon:  10,
		MaxLat:  12,
		MaxLon:  12,
		ZoomMin: 5,
		ZoomMax: 5,
	}

	responsePayload, err := buildOfflineEstimate(requestPayload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if responsePayload.LayerCount != 1 {
		t.Fatalf("expected default layer count 1, got %d", responsePayload.LayerCount)
	}
	if responsePayload.EstimatedBytes <= 0 {
		t.Fatalf("expected estimated bytes > 0, got %d", responsePayload.EstimatedBytes)
	}
}

func TestNormalizeOfflineEstimateRequest_InvalidZoomRange(t *testing.T) {
	requestPayload := offlineEstimateRequest{
		MinLat:  0,
		MinLon:  0,
		MaxLat:  1,
		MaxLon:  1,
		ZoomMin: 10,
		ZoomMax: 2,
	}

	_, err := normalizeOfflineEstimateRequest(requestPayload)
	if err == nil {
		t.Fatal("expected error for invalid zoom range")
	}
}
