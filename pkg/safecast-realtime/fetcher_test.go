package safecastrealtime

import (
	"encoding/json"
	"testing"
)

// TestIsRadiationReading exercises the filtering logic so realtime fetches keep
// only detectors we understand.  This guards the pipeline against Safecast Air
// payloads and unsupported tubes that otherwise pollute the map with zeros.
func TestIsRadiationReading(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		d    devicePayload
		want bool
	}{
		{
			name: "car 7318 cpm",
			d:    devicePayload{Type: "car", Tube: "lnd-7318c", Unit: "lnd_7318c_cpm"},
			want: true,
		},
		{
			name: "air sensor type",
			d:    devicePayload{Type: "AirSensor", Tube: "", Unit: "µSv/h"},
			want: false,
		},
		{
			name: "air descriptor in tube",
			d:    devicePayload{Type: "walk", Tube: "Safecast Air", Unit: "µSv/h"},
			want: false,
		},
		{
			name: "unknown tube",
			d:    devicePayload{Type: "car", Tube: "lnd-78017", Unit: "lnd_78017_cpm"},
			want: false,
		},
		{
			name: "direct microsievert",
			d:    devicePayload{Type: "walk", Tube: "", Unit: "µSv/h"},
			want: true,
		},
		{
			name: "allowed tube via unit",
			d:    devicePayload{Type: "bike", Tube: "", Unit: "lnd_7128_ec"},
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isRadiationReading(tc.d); got != tc.want {
				t.Fatalf("isRadiationReading(%+v)=%v want %v", tc.d, got, tc.want)
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

	jsAlt := `{"service_transport":"car","tube_type":"LND 712","lnd_712_cpm":216}`
	var dAlt devicePayload
	if err := json.Unmarshal([]byte(jsAlt), &dAlt); err != nil {
		t.Fatalf("unmarshal alt: %v", err)
	}
	if dAlt.Type != "car" {
		t.Fatalf("Type=%q want car", dAlt.Type)
	}
	if dAlt.Tube != "LND 712" {
		t.Fatalf("Tube=%q want LND 712", dAlt.Tube)
	}
}
