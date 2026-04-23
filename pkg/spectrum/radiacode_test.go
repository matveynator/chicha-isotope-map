package spectrum

import "testing"

func TestAnalyzeRadiacodeXML(t *testing.T) {
	xmlInput := []byte(`<?xml version="1.0"?>
<ResultDataFile>
  <ResultDataList>
    <ResultData>
      <DeviceConfigReference><Name>RadiaCode-101</Name></DeviceConfigReference>
      <StartTime>2026-04-06T16:03:38</StartTime>
      <EndTime>2026-04-06T16:13:24</EndTime>
      <EnergySpectrum>
        <SerialNumber>RC-101-005020</SerialNumber>
        <MeasurementTime>586</MeasurementTime>
        <EnergyCalibration>
          <Coefficients>
            <Coefficient>-8.075389</Coefficient>
            <Coefficient>2.5978222</Coefficient>
            <Coefficient>0.00052667724</Coefficient>
          </Coefficients>
        </EnergyCalibration>
        <Spectrum>
          <DataPoint>0</DataPoint><DataPoint>3</DataPoint><DataPoint>14</DataPoint><DataPoint>3</DataPoint>
          <DataPoint>2</DataPoint><DataPoint>10</DataPoint><DataPoint>2</DataPoint><DataPoint>0</DataPoint>
          <DataPoint>2</DataPoint><DataPoint>12</DataPoint><DataPoint>2</DataPoint><DataPoint>0</DataPoint>
        </Spectrum>
      </EnergySpectrum>
    </ResultData>
  </ResultDataList>
</ResultDataFile>`)

	analysis, err := AnalyzeRadiacodeXML(xmlInput)
	if err != nil {
		t.Fatalf("AnalyzeRadiacodeXML returned error: %v", err)
	}
	if analysis.Measurement.DeviceSerial != "RC-101-005020" {
		t.Fatalf("unexpected serial: %s", analysis.Measurement.DeviceSerial)
	}
	if analysis.Measurement.StartTime.IsZero() || analysis.Measurement.EndTime.IsZero() {
		t.Fatalf("expected parsed timestamps")
	}
	if len(analysis.DetectedPeaks) == 0 {
		t.Fatalf("expected detected peaks")
	}
}

func TestParseWithKnownDrivers(t *testing.T) {
	xmlInput := []byte(`<ResultDataFile><ResultDataList><ResultData><StartTime>2026-04-06T16:03:38</StartTime><EndTime>2026-04-06T16:13:24</EndTime><EnergySpectrum><SerialNumber>RC-101-005020</SerialNumber><MeasurementTime>586</MeasurementTime><EnergyCalibration><Coefficients><Coefficient>0</Coefficient><Coefficient>1</Coefficient></Coefficients></EnergyCalibration><Spectrum><DataPoint>1</DataPoint><DataPoint>5</DataPoint><DataPoint>1</DataPoint></Spectrum></EnergySpectrum></ResultData></ResultDataList></ResultDataFile>`)

	measurement, err := ParseWithKnownDrivers(xmlInput)
	if err != nil {
		t.Fatalf("ParseWithKnownDrivers returned error: %v", err)
	}
	if measurement.Format != "radiacode-xml" {
		t.Fatalf("unexpected format: %s", measurement.Format)
	}
}

func TestFindClosestMarkerByTime(t *testing.T) {
	markers := []MarkerTimePoint{
		{Date: 1775481600, Lat: 43.27, Lon: 42.49},
		{Date: 1775482000, Lat: 43.28, Lon: 42.50},
		{Date: 1775482600, Lat: 43.29, Lon: 42.51},
	}

	best, ok := FindClosestMarkerByTime(markers, 1775481900, 1775482300, 120)
	if !ok {
		t.Fatalf("expected marker match")
	}
	if best.Marker.Date != 1775482000 {
		t.Fatalf("unexpected marker date: %d", best.Marker.Date)
	}
	if !best.WithinWindow {
		t.Fatalf("expected marker to be inside time window")
	}
}

func TestFindNuclideAlias(t *testing.T) {
	nuclide, ok := FindNuclide("tritium")
	if !ok {
		t.Fatalf("expected tritium alias")
	}
	if nuclide.NuclideID != "H-3" {
		t.Fatalf("unexpected nuclide id: %s", nuclide.NuclideID)
	}

	nuclide, ok = FindNuclide("137Cs")
	if !ok || nuclide.NuclideID != "Cs-137" {
		t.Fatalf("expected normalized Cs-137, got %+v", nuclide)
	}
}
