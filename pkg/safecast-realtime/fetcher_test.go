package safecastrealtime

import (
	"encoding/json"
	"testing"
	"time"
)

// TestConvertIfRadiation exercises the filtering logic so realtime fetches keep
// only detectors that emit radiation readings.  This guards the pipeline
// against Safecast Air payloads that otherwise pollute the map with zeros.
func TestConvertIfRadiation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		d    devicePayload
		want float64
		ok   bool
	}{
		{
			name: "car 7318 cpm",
			d:    devicePayload{ID: "geigiecast:1", Type: "car", Tube: "lnd-7318c", Unit: "lnd_7318c_cpm", Value: 334},
			want: 1,
			ok:   true,
		},
		{
			name: "air sensor type",
			d:    devicePayload{ID: "airsensor:2", Type: "AirSensor", Unit: "µSv/h", Value: 0.5},
			want: 0,
			ok:   false,
		},
		{
			name: "air descriptor in name",
			d:    devicePayload{ID: "geigiecast:3", Type: "walk", Name: "Safecast Air Monitor", Unit: "µSv/h", Value: 0.5},
			want: 0,
			ok:   false,
		},
		{
			name: "direct microsievert",
			d:    devicePayload{ID: "geigiecast:4", Type: "walk", Unit: "µSv/h", Value: 0.42},
			want: 0.42,
			ok:   true,
		},
		{
			name: "unsupported tube",
			d:    devicePayload{ID: "geigiecast:5", Type: "car", Tube: "lnd-78017", Unit: "lnd_78017_cpm", Value: 10},
			want: 0,
			ok:   false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := convertIfRadiation(tc.d)
			if ok != tc.ok {
				t.Fatalf("convertIfRadiation(%+v) ok=%v want %v", tc.d, ok, tc.ok)
			}
			if ok && (got < tc.want-0.0001 || got > tc.want+0.0001) {
				t.Fatalf("convertIfRadiation(%+v)=%v want %v", tc.d, got, tc.want)
			}
		})
	}
}

// TestDevicePayloadUnmarshalTube confirms we extract both the transport and
// tube hints from the flexible JSON feed.  Keeping the parsing logic stable
// prevents regressions when the service advertises multiple metadata forms.
func TestDevicePayloadUnmarshalTube(t *testing.T) {
	t.Parallel()

	js := `{
                "device_urn": "device:123",
                "device_title": "bGeigie #123",
                "service_transport": "walk:lnd-7318c:open",
                "loc_lat": 35.0,
                "loc_lon": 139.0,
                "lnd_7318c_cpm": 42,
                "when_captured": "2024-05-01T12:34:56Z"
        }`

	var d devicePayload
	if err := json.Unmarshal([]byte(js), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Type != "walk" {
		t.Fatalf("Type=%q want walk", d.Type)
	}
	if d.Tube != "lnd-7318c" {
		t.Fatalf("Tube=%q want lnd-7318c", d.Tube)
	}
	if d.Name != "bGeigie #123" {
		t.Fatalf("Name=%q want bGeigie #123", d.Name)
	}

	jsAlt := `{"service_transport":"car","device_name":"bGeigie","value_time":"2024-06-01T00:01:02Z","lnd_712_cpm":216}`
	var dAlt devicePayload
	if err := json.Unmarshal([]byte(jsAlt), &dAlt); err != nil {
		t.Fatalf("unmarshal alt: %v", err)
	}
	if dAlt.Type != "car" {
		t.Fatalf("Type=%q want car", dAlt.Type)
	}
	if dAlt.Name != "bGeigie" {
		t.Fatalf("Name=%q want bGeigie", dAlt.Name)
	}
	wantTime, _ := time.Parse(time.RFC3339, "2024-06-01T00:01:02Z")
	if dAlt.Time != wantTime.Unix() {
		t.Fatalf("Time=%d want %d", dAlt.Time, wantTime.Unix())
	}
}

// TestDevicePayloadMeasurementSelection keeps the measurement picking logic
// predictable so CPS feeds are preferred over CPM, while plain detector fields
// still work as a fallback.
func TestDevicePayloadMeasurementSelection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		json      string
		wantValue float64
		wantUnit  string
	}{
		{
			name:      "prefers cps over cpm",
			json:      `{"device_urn":"device:1","lnd_7318c_cpm":120,"lnd_7318c_cps":2}`,
			wantValue: 2,
			wantUnit:  "lnd_7318c_cps",
		},
		{
			name:      "falls back to raw counts",
			json:      `{"device_urn":"device:2","lnd_7318u":73}`,
			wantValue: 73,
			wantUnit:  "lnd_7318u",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var d devicePayload
			if err := json.Unmarshal([]byte(tc.json), &d); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if d.Value != tc.wantValue {
				t.Fatalf("Value=%v want %v", d.Value, tc.wantValue)
			}
			if d.Unit != tc.wantUnit {
				t.Fatalf("Unit=%q want %q", d.Unit, tc.wantUnit)
			}
		})
	}
}

// TestDevicePayloadMetrics ensures we pick up optional environmental metrics.
func TestDevicePayloadMetrics(t *testing.T) {
	t.Parallel()

	js := `{
                "device_urn": "device:42",
                "loc_lat": 35.0,
                "loc_lon": 139.0,
                "temperature_c": 21.5,
                "humidity": "48",
                "when_captured": "2024-05-01T12:00:00Z"
        }`

	var d devicePayload
	if err := json.Unmarshal([]byte(js), &d); err != nil {
		t.Fatalf("unmarshal metrics: %v", err)
	}
	if d.Metrics == nil {
		t.Fatalf("expected metrics map")
	}
	if v, ok := d.Metrics["temperature_c"]; !ok || v != 21.5 {
		t.Fatalf("temperature_c=%v ok=%v", v, ok)
	}
	if v, ok := d.Metrics["humidity_percent"]; !ok || v != 48 {
		t.Fatalf("humidity_percent=%v ok=%v", v, ok)
	}
}
