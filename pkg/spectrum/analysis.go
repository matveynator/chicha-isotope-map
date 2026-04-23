package spectrum

import (
	"math"
	"sort"
)

// AnalyzeMeasurement runs generic peak detection and isotope matching.
func AnalyzeMeasurement(measurement SpectrumMeasurement) Analysis {
	peaks := findPeaks(measurement.Channels, measurement.Coefficients)
	isotopes := matchIsotopes(peaks, DefaultCatalog())
	composites := buildCompositeModels(peaks, isotopes, 3)
	return Analysis{Measurement: measurement, DetectedPeaks: peaks, Isotopes: isotopes, CompositeModels: composites}
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
	hits := make([]IsotopeHit, 0, 64)
	if len(peaks) == 0 || len(nuclides) == 0 {
		return hits
	}

	for _, nuclide := range nuclides {
		for _, line := range nuclide.RadiationLines() {
			peak, ok := closestPeakForEnergy(peaks, line.EnergyKeV)
			if !ok {
				continue
			}
			delta := math.Abs(peak.Energy - line.EnergyKeV)
			tolerance := 25.0
			if line.RadiationType == "alpha" {
				tolerance = 60.0
			}
			if delta > tolerance {
				continue
			}
			confidence := 1.0 - delta/tolerance
			if confidence < 0 {
				confidence = 0
			}
			hits = append(hits, IsotopeHit{
				Name:          nuclide.DisplayName,
				NuclideID:     nuclide.NuclideID,
				RadiationType: line.RadiationType,
				EnergyKeV:     line.EnergyKeV,
				PeakEnergy:    peak.Energy,
				DeltaKeV:      delta,
				Confidence:    confidence,
				Series:        nuclide.DecaySeries,
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Confidence == hits[j].Confidence {
			return hits[i].DeltaKeV < hits[j].DeltaKeV
		}
		return hits[i].Confidence > hits[j].Confidence
	})

	if len(hits) > 80 {
		return hits[:80]
	}
	return hits
}

func buildCompositeModels(peaks []Peak, hits []IsotopeHit, maxComponents int) []CompositeHit {
	if len(peaks) == 0 || len(hits) == 0 || maxComponents < 2 {
		return nil
	}

	candidates := topNuclideCandidates(hits, 12)
	if len(candidates) < 2 {
		return nil
	}

	combos := enumerateNuclideCombos(candidates, maxComponents)
	if len(combos) == 0 {
		return nil
	}

	resultsChannel := make(chan CompositeHit, len(combos))
	for _, combo := range combos {
		comboCopy := append([]string(nil), combo...)
		go func() {
			resultsChannel <- evaluateCombo(peaks, hits, comboCopy)
		}()
	}

	models := make([]CompositeHit, 0, len(combos))
	for range combos {
		model := <-resultsChannel
		if model.MatchedPeaks == 0 {
			continue
		}
		models = append(models, model)
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].TotalScore == models[j].TotalScore {
			return models[i].ResidualKeV < models[j].ResidualKeV
		}
		return models[i].TotalScore > models[j].TotalScore
	})
	if len(models) > 20 {
		return models[:20]
	}
	return models
}

func topNuclideCandidates(hits []IsotopeHit, limit int) []string {
	scoreByNuclide := make(map[string]float64)
	for _, hit := range hits {
		scoreByNuclide[hit.NuclideID] += hit.Confidence
	}
	type scored struct {
		nuclide string
		score   float64
	}
	rows := make([]scored, 0, len(scoreByNuclide))
	for nuclide, score := range scoreByNuclide {
		rows = append(rows, scored{nuclide: nuclide, score: score})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].score > rows[j].score })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.nuclide)
	}
	return out
}

func enumerateNuclideCombos(nuclides []string, maxComponents int) [][]string {
	combos := make([][]string, 0, 64)
	for size := 2; size <= maxComponents; size++ {
		collectCombos(nuclides, size, 0, nil, &combos)
	}
	return combos
}

func collectCombos(nuclides []string, size, start int, prefix []string, combos *[][]string) {
	if len(prefix) == size {
		item := append([]string(nil), prefix...)
		*combos = append(*combos, item)
		return
	}
	for index := start; index < len(nuclides); index++ {
		prefix = append(prefix, nuclides[index])
		collectCombos(nuclides, size, index+1, prefix, combos)
		prefix = prefix[:len(prefix)-1]
	}
}

func evaluateCombo(peaks []Peak, hits []IsotopeHit, combo []string) CompositeHit {
	inCombo := make(map[string]struct{}, len(combo))
	for _, nuclide := range combo {
		inCombo[nuclide] = struct{}{}
	}
	bestByPeak := make(map[int]IsotopeHit)
	for _, hit := range hits {
		if _, ok := inCombo[hit.NuclideID]; !ok {
			continue
		}
		peakKey := int(math.Round(hit.PeakEnergy * 10))
		current, exists := bestByPeak[peakKey]
		if !exists || hit.Confidence > current.Confidence {
			bestByPeak[peakKey] = hit
		}
	}
	if len(bestByPeak) == 0 {
		return CompositeHit{NuclideIDs: combo}
	}
	var residual float64
	var score float64
	for _, hit := range bestByPeak {
		residual += hit.DeltaKeV
		score += hit.Confidence
	}
	coverage := float64(len(bestByPeak)) / float64(len(peaks))
	return CompositeHit{
		NuclideIDs:   combo,
		Coverage:     coverage,
		ResidualKeV:  residual / float64(len(bestByPeak)),
		TotalScore:   score + coverage,
		MatchedPeaks: len(bestByPeak),
	}
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
