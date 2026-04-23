package spectrum

import (
	"math"
	"sort"
)

// AnalyzeMeasurement runs generic peak detection and isotope matching.
func AnalyzeMeasurement(measurement SpectrumMeasurement) Analysis {
	peaks := findPeaks(measurement.Channels, measurement.Coefficients)
	isotopes := matchIsotopes(peaks, DefaultCatalog())
	return Analysis{Measurement: measurement, DetectedPeaks: peaks, Isotopes: isotopes}
}

// AnalyzeWithKnownDrivers parses with known drivers and analyzes the result.
func AnalyzeWithKnownDrivers(raw []byte) (Analysis, error) {
	measurement, err := ParseWithKnownDrivers(raw)
	if err != nil {
		return Analysis{}, err
	}
	return AnalyzeMeasurement(measurement), nil
}

// AnalyzeRadiacodeXML keeps backward compatibility with the previous API.
func AnalyzeRadiacodeXML(raw []byte) (Analysis, error) {
	driver := RadiacodeXMLDriver{}
	measurement, err := driver.Parse(raw)
	if err != nil {
		return Analysis{}, err
	}
	return AnalyzeMeasurement(measurement), nil
}

// FindClosestMarkerByTime chooses the nearest marker to the spectrum window center.
func FindClosestMarkerByTime(markers []MarkerTimePoint, windowStart, windowEnd, marginSec int64) (MarkerMatch, bool) {
	if len(markers) == 0 {
		return MarkerMatch{}, false
	}
	if windowEnd < windowStart {
		windowStart, windowEnd = windowEnd, windowStart
	}
	windowCenter := windowStart + (windowEnd-windowStart)/2
	best := MarkerMatch{}
	bestSet := false

	for _, marker := range markers {
		delta := marker.Date - windowCenter
		if delta < 0 {
			delta = -delta
		}
		candidate := MarkerMatch{
			Marker:             marker,
			AbsoluteDeltaSec:   delta,
			WithinWindow:       marker.Date >= windowStart && marker.Date <= windowEnd,
			WithinWindowMargin: marker.Date >= windowStart-marginSec && marker.Date <= windowEnd+marginSec,
		}
		if !bestSet || candidate.AbsoluteDeltaSec < best.AbsoluteDeltaSec {
			best = candidate
			bestSet = true
		}
	}
	return best, bestSet
}

func findPeaks(channels []float64, coeffs []float64) []Peak {
	if len(channels) < 3 || len(coeffs) < 2 {
		return nil
	}
	peaks := make([]Peak, 0, 24)
	for channelIndex := 1; channelIndex < len(channels)-1; channelIndex++ {
		left := channels[channelIndex-1]
		middle := channels[channelIndex]
		right := channels[channelIndex+1]
		if middle < 4 {
			continue
		}
		if middle > left && middle >= right {
			energy := coeffs[0] + coeffs[1]*float64(channelIndex)
			if len(coeffs) > 2 {
				energy += coeffs[2] * float64(channelIndex*channelIndex)
			}
			peaks = append(peaks, Peak{Channel: channelIndex, Energy: energy, Counts: middle})
		}
	}
	sort.Slice(peaks, func(i, j int) bool {
		return peaks[i].Counts > peaks[j].Counts
	})
	if len(peaks) > 24 {
		return peaks[:24]
	}
	return peaks
}

func matchIsotopes(peaks []Peak, nuclides []Nuclide) []IsotopeHit {
	hits := make([]IsotopeHit, 0, 32)
	if len(peaks) == 0 || len(nuclides) == 0 {
		return hits
	}

	for _, nuclide := range nuclides {
		for _, gammaLine := range nuclide.GammaLinesKeV {
			peak, ok := closestPeakForEnergy(peaks, gammaLine)
			if !ok {
				continue
			}
			delta := math.Abs(peak.Energy - gammaLine)
			if delta > 25 {
				continue
			}
			confidence := 1.0 - delta/25.0
			if confidence < 0 {
				confidence = 0
			}
			hits = append(hits, IsotopeHit{
				Name:       nuclide.DisplayName,
				NuclideID:  nuclide.NuclideID,
				EnergyKeV:  gammaLine,
				PeakEnergy: peak.Energy,
				DeltaKeV:   delta,
				Confidence: confidence,
				Series:     nuclide.DecaySeries,
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Confidence == hits[j].Confidence {
			return hits[i].DeltaKeV < hits[j].DeltaKeV
		}
		return hits[i].Confidence > hits[j].Confidence
	})

	if len(hits) > 50 {
		return hits[:50]
	}
	return hits
}

func closestPeakForEnergy(peaks []Peak, targetEnergy float64) (Peak, bool) {
	if len(peaks) == 0 {
		return Peak{}, false
	}
	best := peaks[0]
	bestDelta := math.Abs(best.Energy - targetEnergy)
	for _, peak := range peaks[1:] {
		delta := math.Abs(peak.Energy - targetEnergy)
		if delta < bestDelta {
			best = peak
			bestDelta = delta
		}
	}
	return best, true
}
