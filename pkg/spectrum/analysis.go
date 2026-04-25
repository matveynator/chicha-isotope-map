package spectrum

import (
	"math"
	"sort"
	"strings"
)

// AnalyzeMeasurement runs generic peak detection and isotope matching.
func AnalyzeMeasurement(measurement SpectrumMeasurement) Analysis {
	peaks := findPeaks(measurement.Channels, measurement.Coefficients)
	isotopes := matchIsotopes(peaks, DefaultCatalog())
	composites := buildCompositeModels(peaks, isotopes, 3)
	components := estimateSpectrumComponents(peaks)
	groupChecks := evaluatePracticalGroups(peaks)
	explanation := buildPlainLanguageExplanation(components, groupChecks)
	return Analysis{
		Measurement:     measurement,
		DetectedPeaks:   peaks,
		Isotopes:        isotopes,
		CompositeModels: composites,
		Components:      components,
		GroupChecks:     groupChecks,
		Explanation:     explanation,
	}
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

type componentTemplate struct {
	ComponentID  string
	DisplayName  string
	LineEnergies []float64
}

func estimateSpectrumComponents(peaks []Peak) []SpectrumComponent {
	if len(peaks) == 0 {
		return []SpectrumComponent{{ComponentID: "unknown", DisplayName: "Unknown / background", Contribution: 1}}
	}

	templates := []componentTemplate{
		{ComponentID: "k40", DisplayName: "K-40", LineEnergies: []float64{1460.8}},
		{ComponentID: "u_ra_series", DisplayName: "U/Ra series", LineEnergies: []float64{186.2, 295.2, 351.9, 609.3, 1120.3, 1764.5}},
		{ComponentID: "th_series", DisplayName: "Th series", LineEnergies: []float64{238.6, 338.3, 583.2, 911.2, 969.0, 2614.5}},
		{ComponentID: "cs137", DisplayName: "Cs-137", LineEnergies: []float64{661.7}},
		{ComponentID: "co60", DisplayName: "Co-60", LineEnergies: []float64{1173.2, 1332.5}},
		{ComponentID: "am241", DisplayName: "Am-241", LineEnergies: []float64{59.5}},
		{ComponentID: "i123", DisplayName: "I-123", LineEnergies: []float64{159.0}},
		{ComponentID: "tc99m", DisplayName: "Tc-99m", LineEnergies: []float64{140.5}},
	}

	components := make([]SpectrumComponent, 0, len(templates)+1)
	totalScore := 0.0
	for _, template := range templates {
		score := 0.0
		matchedLines := 0
		totalError := 0.0
		for _, lineEnergy := range template.LineEnergies {
			peak, ok := closestPeakForEnergy(peaks, lineEnergy)
			if !ok {
				continue
			}
			delta := math.Abs(peak.Energy - lineEnergy)
			tolerance := 40.0
			if lineEnergy < 200 {
				tolerance = 30.0
			}
			if delta > tolerance {
				continue
			}
			lineScore := (1.0 - delta/tolerance) * math.Sqrt(math.Max(peak.Counts, 1))
			score += lineScore
			matchedLines++
			totalError += delta
		}
		if matchedLines == 0 || score <= 0 {
			continue
		}
		component := SpectrumComponent{
			ComponentID:  template.ComponentID,
			DisplayName:  template.DisplayName,
			Contribution: score,
			MatchedLines: matchedLines,
		}
		component.AverageLineError = totalError / float64(matchedLines)
		components = append(components, component)
		totalScore += score
	}

	if totalScore <= 0 {
		return []SpectrumComponent{{ComponentID: "unknown", DisplayName: "Unknown / background", Contribution: 1}}
	}

	const unknownFloor = 0.08
	scale := 1.0 - unknownFloor
	for index := range components {
		components[index].Contribution = (components[index].Contribution / totalScore) * scale
	}
	components = append(components, SpectrumComponent{
		ComponentID:  "unknown",
		DisplayName:  "Unknown / background",
		Contribution: unknownFloor,
	})

	sort.Slice(components, func(i, j int) bool {
		return components[i].Contribution > components[j].Contribution
	})
	return components
}

type practicalGroupRule struct {
	GroupID       string
	DisplayName   string
	RequiredLines []float64
	MinMatches    int
}

func evaluatePracticalGroups(peaks []Peak) []GroupCheck {
	rules := []practicalGroupRule{
		{GroupID: "k40", DisplayName: "K-40", RequiredLines: []float64{1460.8}, MinMatches: 1},
		{GroupID: "cs137", DisplayName: "Cs-137", RequiredLines: []float64{661.7}, MinMatches: 1},
		{GroupID: "co60", DisplayName: "Co-60", RequiredLines: []float64{1173.2, 1332.5}, MinMatches: 2},
		{GroupID: "u_ra", DisplayName: "U/Ra series", RequiredLines: []float64{186.2, 295.2, 351.9, 609.3}, MinMatches: 2},
		{GroupID: "th232", DisplayName: "Th-232 series", RequiredLines: []float64{238.6, 583.2, 911.2, 2614.5}, MinMatches: 2},
		{GroupID: "am241", DisplayName: "Am-241", RequiredLines: []float64{59.5}, MinMatches: 1},
	}

	checks := make([]GroupCheck, 0, len(rules))
	for _, rule := range rules {
		matched := make([]float64, 0, len(rule.RequiredLines))
		missing := make([]float64, 0, len(rule.RequiredLines))
		for _, lineEnergy := range rule.RequiredLines {
			peak, ok := closestPeakForEnergy(peaks, lineEnergy)
			if !ok {
				missing = append(missing, lineEnergy)
				continue
			}
			tolerance := 35.0
			if lineEnergy < 120 {
				tolerance = 25.0
			}
			if math.Abs(peak.Energy-lineEnergy) <= tolerance {
				matched = append(matched, lineEnergy)
			} else {
				missing = append(missing, lineEnergy)
			}
		}
		confidence := float64(len(matched)) / float64(len(rule.RequiredLines))
		isConfirmed := len(matched) >= rule.MinMatches
		comment := "not confirmed by group lines"
		if isConfirmed {
			comment = "confirmed by group line pattern"
		}
		checks = append(checks, GroupCheck{
			GroupID:      rule.GroupID,
			DisplayName:  rule.DisplayName,
			MatchedLines: matched,
			MissingLines: missing,
			Confidence:   confidence,
			IsConfirmed:  isConfirmed,
			Comment:      comment,
		})
	}
	sort.Slice(checks, func(i, j int) bool {
		if checks[i].Confidence == checks[j].Confidence {
			return checks[i].DisplayName < checks[j].DisplayName
		}
		return checks[i].Confidence > checks[j].Confidence
	})
	return checks
}

func buildPlainLanguageExplanation(components []SpectrumComponent, groupChecks []GroupCheck) string {
	if len(components) == 0 {
		return "Spectrum does not contain enough stable evidence for component interpretation."
	}

	leading := make([]string, 0, 2)
	for _, component := range components {
		if component.ComponentID == "unknown" {
			continue
		}
		percent := int(math.Round(component.Contribution * 100))
		if percent < 8 {
			continue
		}
		leading = append(leading, component.DisplayName)
		if len(leading) == 2 {
			break
		}
	}

	confirmed := make([]string, 0, 3)
	rejected := make([]string, 0, 3)
	for _, check := range groupChecks {
		if check.IsConfirmed {
			confirmed = append(confirmed, check.DisplayName)
			continue
		}
		if check.Confidence < 0.35 {
			rejected = append(rejected, check.DisplayName)
		}
	}

	sentenceParts := make([]string, 0, 3)
	if len(leading) > 0 {
		sentenceParts = append(sentenceParts, "Rise is best explained by "+joinHumanList(leading)+".")
	}
	if len(confirmed) > 0 {
		sentenceParts = append(sentenceParts, "Confirmed groups: "+joinHumanList(confirmed)+".")
	}
	if len(rejected) > 0 {
		sentenceParts = append(sentenceParts, "Not confirmed by key lines: "+joinHumanList(rejected)+".")
	}
	if len(sentenceParts) == 0 {
		return "Spectrum fit remains uncertain; key group lines are incomplete."
	}
	return strings.Join(sentenceParts, " ")
}

func joinHumanList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0]
	}
	if len(items) == 2 {
		return items[0] + " and " + items[1]
	}
	return items[0] + ", " + items[1] + ", and " + items[2]
}
