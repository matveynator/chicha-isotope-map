package safecastrealtime

import (
	"math"
	"testing"
)

// TestFromRealtime exercises the supported realtime conversions so a future
// refactor will not accidentally remove detector specific calibration factors.
func TestFromRealtime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value float64
		unit  string
		want  float64
		ok    bool
	}{
		{name: "already microsievert", value: 0.42, unit: "µSv/h", want: 0.42, ok: true},
		{name: "ascii microsievert", value: 1.5, unit: "uSv/h", want: 1.5, ok: true},
		{name: "lnd 7317 cpm", value: 668, unit: "lnd_7317", want: 668 / 334.0, ok: true},
		{name: "lnd 7318 cps", value: 334.0 / 60.0, unit: "lnd-7318-cps", want: 1, ok: true},
		{name: "lnd 7318 cps underscore", value: 334.0 / 60.0, unit: "lnd_7318c_cps", want: 1, ok: true},
		{name: "lnd 7318 cpm with suffix", value: 3340, unit: "lnd-7318-cpm", want: 3340 / 334.0, ok: true},
		{name: "lnd 712 cpm", value: 216, unit: "lnd_712", want: 216 / 108.0, ok: true},
		{name: "lnd 7128 ec", value: 216, unit: "lnd_7128_ec", want: 216 / 108.0, ok: true},
		{name: "lnd 7318 fallback", value: 53, unit: "lnd_7318u", want: 53 / 334.0, ok: true},
		{name: "unknown detector", value: 123, unit: "lnd_78017", want: 0, ok: false},
		{name: "non positive", value: 0, unit: "lnd_7317", want: 0, ok: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := FromRealtime(tc.value, tc.unit)
			if ok != tc.ok {
				t.Fatalf("FromRealtime(%f,%q) ok=%t want %t", tc.value, tc.unit, ok, tc.ok)
			}
			if !ok {
				return
			}
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("FromRealtime(%f,%q)=%f want %f", tc.value, tc.unit, got, tc.want)
			}
		})
	}
}
