package jrcremrealtime

import "testing"

func TestFromRealtime(t *testing.T) {
	if got, ok := FromRealtime(250, "nSv/h"); !ok || got != 0.25 {
		t.Fatalf("nSv conversion failed: got=%v ok=%v", got, ok)
	}
	if got, ok := FromRealtime(0.12, "µSv/h"); !ok || got != 0.12 {
		t.Fatalf("µSv passthrough failed: got=%v ok=%v", got, ok)
	}
	if _, ok := FromRealtime(1, "cpm"); ok {
		t.Fatal("expected unsupported unit to fail")
	}
}

func TestDecodeStations(t *testing.T) {
	payload := []byte(`[{"id":"A1","name":"Alpha","latitude":50.1,"longitude":14.4,"nsv":95,"date":"2026-02-18T10:00:00Z"}]`)
	stations, err := decodeStations(payload)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(stations) != 1 {
		t.Fatalf("unexpected station count: %d", len(stations))
	}
	if stations[0].ID != "A1" || stations[0].ValueNSvH != 95 || stations[0].MeasuredAt == 0 {
		t.Fatalf("unexpected station payload: %+v", stations[0])
	}
}

func TestParseTimestampCompactUTC(t *testing.T) {
	ts := parseTimestamp("20260218102644")
	if ts == 0 {
		t.Fatal("expected compact datetime to parse")
	}
	if ts < 1_700_000_000 {
		t.Fatalf("unexpected old timestamp: %d", ts)
	}
}

func TestNormalizeCoordinates(t *testing.T) {
	lat, lon, ok := normalizeCoordinates(120, 45)
	if !ok {
		t.Fatal("expected swapped coordinates to be accepted")
	}
	if lat != 45 || lon != 120 {
		t.Fatalf("unexpected normalized pair: %f,%f", lat, lon)
	}

	if _, _, ok := normalizeCoordinates(999, 999); ok {
		t.Fatal("expected invalid coordinates to be rejected")
	}
}
