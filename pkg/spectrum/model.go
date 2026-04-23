package spectrum

import "time"

// MarkerTimePoint keeps the minimum track fields needed to link a spectrum to map points.
type MarkerTimePoint struct {
	Date int64
	Lat  float64
	Lon  float64
}

// SpectrumMeasurement is the common model produced by all instrument drivers.
// Keeping one shape lets isotope analysis stay device-agnostic.
type SpectrumMeasurement struct {
	Format         string
	DeviceName     string
	DeviceSerial   string
	StartTime      time.Time
	EndTime        time.Time
	MeasurementSec int64
	Channels       []float64
	Coefficients   []float64
}

// Peak describes one identified spectral peak candidate.
type Peak struct {
	Channel int
	Energy  float64
	Counts  float64
}

// IsotopeHit keeps a matched isotope candidate with confidence score.
type IsotopeHit struct {
	Name       string
	NuclideID  string
	EnergyKeV  float64
	PeakEnergy float64
	DeltaKeV   float64
	Confidence float64
	Series     string
}

// Analysis bundles parsed spectrum and lookup results.
type Analysis struct {
	Measurement   SpectrumMeasurement
	DetectedPeaks []Peak
	Isotopes      []IsotopeHit
}

// MarkerMatch reports which marker is closest in time to the spectrum window.
type MarkerMatch struct {
	Marker             MarkerTimePoint
	AbsoluteDeltaSec   int64
	WithinWindow       bool
	WithinWindowMargin bool
}
