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

	nuclide, ok = FindNuclide("Tc-99m")
	if !ok || nuclide.NuclideID != "Tc-99m" {
		t.Fatalf("expected normalized Tc-99m, got %+v", nuclide)
	}

	nuclide, ok = FindNuclide("99mTc")
	if !ok || nuclide.NuclideID != "Tc-99m" {
		t.Fatalf("expected normalized 99mTc -> Tc-99m, got %+v", nuclide)
	}

	nuclide, ok = FindNuclide("Pa-234m")
	if !ok || nuclide.NuclideID != "Pa-234m" {
		t.Fatalf("expected normalized Pa-234m, got %+v", nuclide)
	}
}

func TestRadiationTypesAndCompositeModel(t *testing.T) {
	measurement := SpectrumMeasurement{
		Coefficients: []float64{0, 1},
		Channels:     make([]float64, 1700),
	}
	measurement.Channels[59] = 10 // Am-241 gamma 59.5
	measurement.Channels[60] = 1
	measurement.Channels[661] = 9 // Cs-137 gamma 661.7
	measurement.Channels[662] = 1
	measurement.Channels[1460] = 8 // K-40 gamma 1460.8 (clipped by channel size in this synthetic test)

	analysis := AnalyzeMeasurement(measurement)
	if len(analysis.Isotopes) == 0 {
		t.Fatalf("expected isotope hits")
	}
	foundGamma := false
	for _, hit := range analysis.Isotopes {
		if hit.RadiationType == "gamma" {
			foundGamma = true
			break
		}
	}
	if !foundGamma {
		t.Fatalf("expected gamma radiation type in hits")
	}
	if len(analysis.CompositeModels) == 0 {
		t.Fatalf("expected composite models for mixed spectrum")
	}
	if len(analysis.Components) == 0 {
		t.Fatalf("expected coarse component estimation")
	}
	if analysis.Components[0].Contribution <= 0 {
		t.Fatalf("expected non-zero leading component contribution")
	}
	if len(analysis.GroupChecks) == 0 {
		t.Fatalf("expected practical group checks")
	}
	if analysis.Explanation == "" {
		t.Fatalf("expected plain-language explanation")
	}
}

func TestCatalogContainsRequiredImportIsotopes(t *testing.T) {
	requiredNuclideIDs := []string{
		"H-3", "C-14", "Be-7", "Be-10", "Na-22", "Na-24", "Mg-28", "Al-26", "Si-31", "P-32",
		"P-33", "S-35", "Cl-36", "Ar-37", "Ar-39", "Ar-41", "K-40", "K-42", "Ca-45", "Ca-47",
		"Sc-46", "Sc-47", "Sc-48", "Ti-44", "V-48", "Cr-51", "Mn-52", "Mn-54", "Mn-56", "Fe-55",
		"Fe-59", "Co-56", "Co-57", "Co-58", "Co-60", "Ni-59", "Ni-63", "Cu-64", "Cu-67", "Zn-65",
		"Ga-67", "Ga-68", "Ge-68", "As-72", "As-74", "As-76", "Se-75", "Br-82", "Kr-85", "Rb-86",
		"Rb-87", "Sr-85", "Sr-89", "Sr-90", "Y-88", "Y-90", "Zr-89", "Zr-95", "Nb-94", "Nb-95",
		"Mo-99", "Tc-99", "Tc-99m", "Ru-103", "Ru-106", "Rh-106", "Pd-103", "Ag-108m", "Ag-110m", "Cd-109",
		"Cd-115m", "In-111", "In-113m", "Sn-113", "Sn-117m", "Sn-125", "Sb-122", "Sb-124", "Sb-125", "Te-125m",
		"Te-127m", "Te-129m", "Te-132", "I-123", "I-125", "I-129", "I-131", "I-132", "I-133", "I-135",
		"Xe-131m", "Xe-133", "Xe-135", "Cs-134", "Cs-135", "Cs-137", "Ba-133", "Ba-140", "La-140", "Ce-139",
		"Ce-141", "Ce-144", "Pr-144", "Nd-147", "Pm-147", "Pm-148m", "Sm-151", "Sm-153", "Eu-152", "Eu-154",
		"Eu-155", "Gd-153", "Tb-160", "Dy-165", "Ho-166", "Er-169", "Tm-170", "Yb-169", "Lu-176", "Lu-177",
		"Hf-181", "Ta-182", "W-181", "W-185", "W-187", "Re-186", "Re-188", "Ir-192", "Au-198", "Hg-203",
		"Tl-201", "Pb-210", "Bi-207", "Bi-210", "Po-210", "Ra-223", "Ra-224", "Ra-226", "Ra-228", "Ac-225",
		"Ac-227", "Ac-228", "Th-227", "Th-228", "Th-230", "Th-232", "Pa-231", "Pa-234m", "U-232", "U-233",
		"U-234", "U-235", "U-238", "Np-237", "Np-239", "Pu-238", "Pu-239", "Pu-240", "Pu-241", "Am-241",
		"Am-243", "Cm-242", "Cm-244", "Cf-252",
	}

	missing := make([]string, 0)
	for _, nuclideID := range requiredNuclideIDs {
		_, found := FindNuclide(nuclideID)
		if !found {
			missing = append(missing, nuclideID)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("required nuclides missing from catalog: %v", missing)
	}
}
