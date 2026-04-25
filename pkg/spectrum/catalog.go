package spectrum

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Nuclide stores isotope metadata independent from vendor format.
type Nuclide struct {
	NuclideID     string
	DisplayName   string
	Element       string
	MassNumber    int
	HalfLife      string
	DecaySeries   string
	Category      string
	TypicalSource string
	Accumulation  string
	Scintillator  string
	AlphaLinesKeV []float64
	BetaMaxKeV    []float64
	GammaLinesKeV []float64
}

// RadiationLine describes one emission line that can participate in spectral matching.
type RadiationLine struct {
	RadiationType string
	EnergyKeV     float64
}

func (nuclide Nuclide) RadiationLines() []RadiationLine {
	lines := make([]RadiationLine, 0, len(nuclide.AlphaLinesKeV)+len(nuclide.BetaMaxKeV)+len(nuclide.GammaLinesKeV))
	for _, energy := range nuclide.AlphaLinesKeV {
		lines = append(lines, RadiationLine{RadiationType: "alpha", EnergyKeV: energy})
	}
	for _, energy := range nuclide.BetaMaxKeV {
		lines = append(lines, RadiationLine{RadiationType: "beta", EnergyKeV: energy})
	}
	for _, energy := range nuclide.GammaLinesKeV {
		lines = append(lines, RadiationLine{RadiationType: "gamma", EnergyKeV: energy})
	}
	return lines
}

// DefaultCatalog returns a broad catalog with natural chains, NORM and key artificial isotopes.
func DefaultCatalog() []Nuclide {
	out := make([]Nuclide, 0, len(defaultNuclides))
	out = append(out, defaultNuclides...)
	sort.Slice(out, func(i, j int) bool { return out[i].NuclideID < out[j].NuclideID })
	return out
}

// FindNuclide normalizes common spellings: "Cs-137", "137Cs", "tritium".
func FindNuclide(input string) (Nuclide, bool) {
	normalized, ok := normalizeNuclideToken(input)
	if !ok {
		return Nuclide{}, false
	}
	for _, nuclide := range defaultNuclides {
		if nuclide.NuclideID == normalized {
			return nuclide, true
		}
	}
	return Nuclide{}, false
}

func normalizeNuclideToken(input string) (string, bool) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "" {
		return "", false
	}
	aliases := map[string]string{
		"tritium":      "H-3",
		"radiocarbon":  "C-14",
		"potassium-40": "K-40",
		"radon":        "Rn-222",
	}
	if alias, ok := aliases[trimmed]; ok {
		return alias, true
	}

	cleaned := strings.NewReplacer(" ", "", "_", "", "–", "-", ".", "").Replace(trimmed)
	if strings.Contains(cleaned, "-") {
		parts := strings.Split(cleaned, "-")
		if len(parts) != 2 {
			return "", false
		}
		element := strings.Title(parts[0])
		mass, metastableSuffix, err := parseMassAndMetastableSuffix(parts[1])
		if err != nil {
			return "", false
		}
		return fmt.Sprintf("%s-%d%s", element, mass, metastableSuffix), true
	}

	firstDigit := -1
	for i, r := range cleaned {
		if r >= '0' && r <= '9' {
			firstDigit = i
			break
		}
	}
	if firstDigit == -1 {
		return "", false
	}

	if firstDigit == 0 {
		mass, metastableSuffix, element, err := parseLeadingMassNotation(cleaned)
		if err != nil {
			return "", false
		}
		if element == "" {
			return "", false
		}
		return fmt.Sprintf("%s-%d%s", element, mass, metastableSuffix), true
	}

	element := strings.Title(cleaned[:firstDigit])
	mass, metastableSuffix, err := parseMassAndMetastableSuffix(cleaned[firstDigit:])
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("%s-%d%s", element, mass, metastableSuffix), true
}

func parseMassAndMetastableSuffix(token string) (int, string, error) {
	if token == "" {
		return 0, "", fmt.Errorf("empty mass token")
	}

	digitEnd := 0
	for digitEnd < len(token) && token[digitEnd] >= '0' && token[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == 0 {
		return 0, "", fmt.Errorf("missing mass number")
	}

	mass, err := strconv.Atoi(token[:digitEnd])
	if err != nil {
		return 0, "", err
	}
	if digitEnd == len(token) {
		return mass, "", nil
	}

	suffix := strings.ToLower(token[digitEnd:])
	if suffix != "m" {
		return 0, "", fmt.Errorf("unsupported mass suffix")
	}
	return mass, suffix, nil
}

func splitElementAndMetastableSuffix(token string) (string, string, error) {
	if token == "" {
		return "", "", fmt.Errorf("empty element token")
	}

	metastableSuffix := ""
	elementToken := token
	if strings.HasSuffix(token, "m") {
		elementToken = token[:len(token)-1]
		metastableSuffix = "m"
	}
	if elementToken == "" {
		return "", "", fmt.Errorf("missing element symbol")
	}
	for _, symbolRune := range elementToken {
		if !unicode.IsLetter(symbolRune) {
			return "", "", fmt.Errorf("invalid element symbol")
		}
	}

	return strings.Title(elementToken), metastableSuffix, nil
}

func parseLeadingMassNotation(token string) (int, string, string, error) {
	digitEnd := 0
	for digitEnd < len(token) && token[digitEnd] >= '0' && token[digitEnd] <= '9' {
		digitEnd++
	}
	if digitEnd == 0 || digitEnd == len(token) {
		return 0, "", "", fmt.Errorf("invalid leading mass notation")
	}

	metastableSuffix := ""
	elementStart := digitEnd
	if token[elementStart] == 'm' {
		metastableSuffix = "m"
		elementStart++
		if elementStart == len(token) {
			return 0, "", "", fmt.Errorf("missing element symbol")
		}
	}

	mass, err := strconv.Atoi(token[:digitEnd])
	if err != nil {
		return 0, "", "", err
	}
	element, extraMetastableSuffix, err := splitElementAndMetastableSuffix(token[elementStart:])
	if err != nil {
		return 0, "", "", err
	}
	if metastableSuffix == "" {
		metastableSuffix = extraMetastableSuffix
	}
	return mass, metastableSuffix, element, nil
}

var defaultNuclides = []Nuclide{
	{NuclideID: "H-3", DisplayName: "Tritium", Element: "H", MassNumber: 3, HalfLife: "12.32 y", Category: "cosmogenic", TypicalSource: "water, tritium signs", BetaMaxKeV: []float64{18.6}},
	{NuclideID: "C-14", DisplayName: "Carbon-14", Element: "C", MassNumber: 14, HalfLife: "5730 y", Category: "cosmogenic", BetaMaxKeV: []float64{156.5}},
	{NuclideID: "K-40", DisplayName: "Potassium-40", Element: "K", MassNumber: 40, HalfLife: "1.248e9 y", Category: "NORM", TypicalSource: "fertilizers, food, concrete", GammaLinesKeV: []float64{1460.8}},
	{NuclideID: "Rb-87", DisplayName: "Rubidium-87", Element: "Rb", MassNumber: 87, HalfLife: "4.88e10 y", Category: "NORM"},
	{NuclideID: "La-138", DisplayName: "Lanthanum-138", Element: "La", MassNumber: 138, HalfLife: "1.02e11 y", Category: "NORM", GammaLinesKeV: []float64{1435.8}},
	{NuclideID: "Sm-147", DisplayName: "Samarium-147", Element: "Sm", MassNumber: 147, HalfLife: "1.06e11 y", Category: "NORM"},
	{NuclideID: "Lu-176", DisplayName: "Lutetium-176", Element: "Lu", MassNumber: 176, HalfLife: "3.78e10 y", Category: "NORM", GammaLinesKeV: []float64{88.3, 202.9, 306.8}},
	{NuclideID: "Re-187", DisplayName: "Rhenium-187", Element: "Re", MassNumber: 187, HalfLife: "4.12e10 y", Category: "NORM"},
	{NuclideID: "In-115", DisplayName: "Indium-115", Element: "In", MassNumber: 115, HalfLife: "4.41e14 y", Category: "NORM"},
	{NuclideID: "V-50", DisplayName: "Vanadium-50", Element: "V", MassNumber: 50, HalfLife: "1.4e17 y", Category: "NORM"},
	{NuclideID: "Bi-209", DisplayName: "Bismuth-209", Element: "Bi", MassNumber: 209, HalfLife: "1.9e19 y", Category: "NORM"},

	// Thorium-232 decay series (4n)
	{NuclideID: "Th-232", DisplayName: "Thorium-232", Element: "Th", MassNumber: 232, HalfLife: "1.405e10 y", DecaySeries: "Th-232", Category: "NORM", TypicalSource: "monazite sands, welding electrodes"},
	{NuclideID: "Ra-228", DisplayName: "Radium-228", Element: "Ra", MassNumber: 228, HalfLife: "5.75 y", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Ac-228", DisplayName: "Actinium-228", Element: "Ac", MassNumber: 228, HalfLife: "6.15 h", DecaySeries: "Th-232", Category: "NORM", GammaLinesKeV: []float64{338.3, 911.2, 969.0}},
	{NuclideID: "Th-228", DisplayName: "Thorium-228", Element: "Th", MassNumber: 228, HalfLife: "1.91 y", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Ra-224", DisplayName: "Radium-224", Element: "Ra", MassNumber: 224, HalfLife: "3.66 d", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Rn-220", DisplayName: "Radon-220", Element: "Rn", MassNumber: 220, HalfLife: "55.6 s", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Po-216", DisplayName: "Polonium-216", Element: "Po", MassNumber: 216, HalfLife: "0.145 s", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Pb-212", DisplayName: "Lead-212", Element: "Pb", MassNumber: 212, HalfLife: "10.64 h", DecaySeries: "Th-232", Category: "NORM", GammaLinesKeV: []float64{238.6}},
	{NuclideID: "Bi-212", DisplayName: "Bismuth-212", Element: "Bi", MassNumber: 212, HalfLife: "60.6 min", DecaySeries: "Th-232", Category: "NORM", GammaLinesKeV: []float64{727.3}},
	{NuclideID: "Tl-208", DisplayName: "Thallium-208", Element: "Tl", MassNumber: 208, HalfLife: "3.05 min", DecaySeries: "Th-232", Category: "NORM", GammaLinesKeV: []float64{583.2, 2614.5}},
	{NuclideID: "Po-212", DisplayName: "Polonium-212", Element: "Po", MassNumber: 212, HalfLife: "0.3 us", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Po-208", DisplayName: "Polonium-208", Element: "Po", MassNumber: 208, HalfLife: "2.9 y", DecaySeries: "Th-232", Category: "NORM"},
	{NuclideID: "Pb-208", DisplayName: "Lead-208", Element: "Pb", MassNumber: 208, HalfLife: "stable", DecaySeries: "Th-232", Category: "stable-end"},

	// Uranium-238 decay series (4n+2)
	{NuclideID: "U-238", DisplayName: "Uranium-238", Element: "U", MassNumber: 238, HalfLife: "4.468e9 y", DecaySeries: "U-238", Category: "NORM", TypicalSource: "granite, phosphogypsum"},
	{NuclideID: "Th-234", DisplayName: "Thorium-234", Element: "Th", MassNumber: 234, HalfLife: "24.1 d", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "Pa-234m", DisplayName: "Protactinium-234m", Element: "Pa", MassNumber: 234, HalfLife: "1.17 min", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "U-234", DisplayName: "Uranium-234", Element: "U", MassNumber: 234, HalfLife: "2.455e5 y", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "Th-230", DisplayName: "Thorium-230", Element: "Th", MassNumber: 230, HalfLife: "75380 y", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "Ra-226", DisplayName: "Radium-226", Element: "Ra", MassNumber: 226, HalfLife: "1600 y", DecaySeries: "U-238", Category: "NORM", AlphaLinesKeV: []float64{4784.3}, GammaLinesKeV: []float64{186.2}},
	{NuclideID: "Rn-222", DisplayName: "Radon-222", Element: "Rn", MassNumber: 222, HalfLife: "3.82 d", DecaySeries: "U-238", Category: "NORM", TypicalSource: "indoor air"},
	{NuclideID: "Po-218", DisplayName: "Polonium-218", Element: "Po", MassNumber: 218, HalfLife: "3.1 min", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "Pb-214", DisplayName: "Lead-214", Element: "Pb", MassNumber: 214, HalfLife: "26.8 min", DecaySeries: "U-238", Category: "NORM", GammaLinesKeV: []float64{295.2, 351.9}},
	{NuclideID: "Bi-214", DisplayName: "Bismuth-214", Element: "Bi", MassNumber: 214, HalfLife: "19.9 min", DecaySeries: "U-238", Category: "NORM", GammaLinesKeV: []float64{609.3, 1120.3, 1764.5}},
	{NuclideID: "Po-214", DisplayName: "Polonium-214", Element: "Po", MassNumber: 214, HalfLife: "164 us", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "Pb-210", DisplayName: "Lead-210", Element: "Pb", MassNumber: 210, HalfLife: "22.3 y", DecaySeries: "U-238", Category: "NORM", GammaLinesKeV: []float64{46.5}},
	{NuclideID: "Bi-210", DisplayName: "Bismuth-210", Element: "Bi", MassNumber: 210, HalfLife: "5.01 d", DecaySeries: "U-238", Category: "NORM"},
	{NuclideID: "Po-210", DisplayName: "Polonium-210", Element: "Po", MassNumber: 210, HalfLife: "138.4 d", DecaySeries: "U-238", Category: "NORM", AlphaLinesKeV: []float64{5304.5}},
	{NuclideID: "Pb-206", DisplayName: "Lead-206", Element: "Pb", MassNumber: 206, HalfLife: "stable", DecaySeries: "U-238", Category: "stable-end"},

	// Uranium-235 / actinium series (4n+3)
	{NuclideID: "U-235", DisplayName: "Uranium-235", Element: "U", MassNumber: 235, HalfLife: "7.04e8 y", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Th-231", DisplayName: "Thorium-231", Element: "Th", MassNumber: 231, HalfLife: "25.5 h", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Pa-231", DisplayName: "Protactinium-231", Element: "Pa", MassNumber: 231, HalfLife: "32760 y", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Ac-227", DisplayName: "Actinium-227", Element: "Ac", MassNumber: 227, HalfLife: "21.8 y", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Th-227", DisplayName: "Thorium-227", Element: "Th", MassNumber: 227, HalfLife: "18.7 d", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Ra-223", DisplayName: "Radium-223", Element: "Ra", MassNumber: 223, HalfLife: "11.4 d", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Rn-219", DisplayName: "Radon-219", Element: "Rn", MassNumber: 219, HalfLife: "3.96 s", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Po-215", DisplayName: "Polonium-215", Element: "Po", MassNumber: 215, HalfLife: "1.78 ms", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Pb-211", DisplayName: "Lead-211", Element: "Pb", MassNumber: 211, HalfLife: "36.1 min", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Bi-211", DisplayName: "Bismuth-211", Element: "Bi", MassNumber: 211, HalfLife: "2.14 min", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Tl-207", DisplayName: "Thallium-207", Element: "Tl", MassNumber: 207, HalfLife: "4.77 min", DecaySeries: "U-235", Category: "NORM"},
	{NuclideID: "Pb-207", DisplayName: "Lead-207", Element: "Pb", MassNumber: 207, HalfLife: "stable", DecaySeries: "U-235", Category: "stable-end"},

	// Neptunium series (4n+1)
	{NuclideID: "Np-237", DisplayName: "Neptunium-237", Element: "Np", MassNumber: 237, HalfLife: "2.14e6 y", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Pa-233", DisplayName: "Protactinium-233", Element: "Pa", MassNumber: 233, HalfLife: "27 d", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "U-233", DisplayName: "Uranium-233", Element: "U", MassNumber: 233, HalfLife: "1.592e5 y", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Th-229", DisplayName: "Thorium-229", Element: "Th", MassNumber: 229, HalfLife: "7932 y", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Ra-225", DisplayName: "Radium-225", Element: "Ra", MassNumber: 225, HalfLife: "14.9 d", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Ac-225", DisplayName: "Actinium-225", Element: "Ac", MassNumber: 225, HalfLife: "10 d", DecaySeries: "Np-237", Category: "medical"},
	{NuclideID: "Fr-221", DisplayName: "Francium-221", Element: "Fr", MassNumber: 221, HalfLife: "4.8 min", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "At-217", DisplayName: "Astatine-217", Element: "At", MassNumber: 217, HalfLife: "32 ms", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Bi-213", DisplayName: "Bismuth-213", Element: "Bi", MassNumber: 213, HalfLife: "45.6 min", DecaySeries: "Np-237", Category: "medical", GammaLinesKeV: []float64{440.5}},
	{NuclideID: "Po-213", DisplayName: "Polonium-213", Element: "Po", MassNumber: 213, HalfLife: "4.2 us", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Tl-209", DisplayName: "Thallium-209", Element: "Tl", MassNumber: 209, HalfLife: "2.2 min", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Pb-209", DisplayName: "Lead-209", Element: "Pb", MassNumber: 209, HalfLife: "3.25 h", DecaySeries: "Np-237", Category: "artificial"},
	{NuclideID: "Bi-209m", DisplayName: "Bismuth-209m", Element: "Bi", MassNumber: 209, HalfLife: "stable", DecaySeries: "Np-237", Category: "stable-end"},

	// Common environmental and industrial isotopes
	{NuclideID: "Be-7", DisplayName: "Beryllium-7", Element: "Be", MassNumber: 7, HalfLife: "53.2 d", Category: "cosmogenic", GammaLinesKeV: []float64{477.6}},
	{NuclideID: "Na-22", DisplayName: "Sodium-22", Element: "Na", MassNumber: 22, HalfLife: "2.6 y", Category: "industrial", GammaLinesKeV: []float64{511.0, 1274.5}},
	{NuclideID: "Na-24", DisplayName: "Sodium-24", Element: "Na", MassNumber: 24, HalfLife: "15 h", Category: "reactor", GammaLinesKeV: []float64{1368.6, 2754.0}},
	{NuclideID: "Co-57", DisplayName: "Cobalt-57", Element: "Co", MassNumber: 57, HalfLife: "271.8 d", Category: "calibration", GammaLinesKeV: []float64{122.1, 136.5}},
	{NuclideID: "Co-58", DisplayName: "Cobalt-58", Element: "Co", MassNumber: 58, HalfLife: "70.9 d", Category: "activation", GammaLinesKeV: []float64{810.8}},
	{NuclideID: "Co-60", DisplayName: "Cobalt-60", Element: "Co", MassNumber: 60, HalfLife: "5.27 y", Category: "industrial", GammaLinesKeV: []float64{1173.2, 1332.5}},
	{NuclideID: "Zn-65", DisplayName: "Zinc-65", Element: "Zn", MassNumber: 65, HalfLife: "244 d", Category: "activation", GammaLinesKeV: []float64{1115.5}},
	{NuclideID: "Se-75", DisplayName: "Selenium-75", Element: "Se", MassNumber: 75, HalfLife: "119.8 d", Category: "industrial", GammaLinesKeV: []float64{136.0, 264.7}},
	{NuclideID: "Kr-85", DisplayName: "Krypton-85", Element: "Kr", MassNumber: 85, HalfLife: "10.7 y", Category: "reprocessing"},
	{NuclideID: "Sr-89", DisplayName: "Strontium-89", Element: "Sr", MassNumber: 89, HalfLife: "50.5 d", Category: "medical"},
	{NuclideID: "Sr-90", DisplayName: "Strontium-90", Element: "Sr", MassNumber: 90, HalfLife: "28.8 y", Category: "fallout", BetaMaxKeV: []float64{546.0}},
	{NuclideID: "Y-90", DisplayName: "Yttrium-90", Element: "Y", MassNumber: 90, HalfLife: "64 h", Category: "medical", BetaMaxKeV: []float64{2280.1}},
	{NuclideID: "Zr-95", DisplayName: "Zirconium-95", Element: "Zr", MassNumber: 95, HalfLife: "64 d", Category: "fission", GammaLinesKeV: []float64{724.2, 756.7}},
	{NuclideID: "Nb-95", DisplayName: "Niobium-95", Element: "Nb", MassNumber: 95, HalfLife: "35 d", Category: "fission", GammaLinesKeV: []float64{765.8}},
	{NuclideID: "Mo-99", DisplayName: "Molybdenum-99", Element: "Mo", MassNumber: 99, HalfLife: "66 h", Category: "medical", GammaLinesKeV: []float64{739.5}},
	{NuclideID: "Tc-99m", DisplayName: "Technetium-99m", Element: "Tc", MassNumber: 99, HalfLife: "6 h", Category: "medical", GammaLinesKeV: []float64{140.5}},
	{NuclideID: "Ru-103", DisplayName: "Ruthenium-103", Element: "Ru", MassNumber: 103, HalfLife: "39.3 d", Category: "fission", GammaLinesKeV: []float64{497.1}},
	{NuclideID: "Ru-106", DisplayName: "Ruthenium-106", Element: "Ru", MassNumber: 106, HalfLife: "373.6 d", Category: "fission"},
	{NuclideID: "Ag-110m", DisplayName: "Silver-110m", Element: "Ag", MassNumber: 110, HalfLife: "249.8 d", Category: "fission", GammaLinesKeV: []float64{657.8, 884.7, 937.5}},
	{NuclideID: "Cd-109", DisplayName: "Cadmium-109", Element: "Cd", MassNumber: 109, HalfLife: "462 d", Category: "calibration", GammaLinesKeV: []float64{88.0}},
	{NuclideID: "Sb-124", DisplayName: "Antimony-124", Element: "Sb", MassNumber: 124, HalfLife: "60.2 d", Category: "activation", GammaLinesKeV: []float64{602.7, 1690.9}},
	{NuclideID: "Sb-125", DisplayName: "Antimony-125", Element: "Sb", MassNumber: 125, HalfLife: "2.76 y", Category: "fission", GammaLinesKeV: []float64{427.9, 600.6}},
	{NuclideID: "I-123", DisplayName: "Iodine-123", Element: "I", MassNumber: 123, HalfLife: "13.2 h", Category: "medical", GammaLinesKeV: []float64{159.0}},
	{NuclideID: "I-125", DisplayName: "Iodine-125", Element: "I", MassNumber: 125, HalfLife: "59.4 d", Category: "medical", GammaLinesKeV: []float64{35.5}},
	{NuclideID: "I-129", DisplayName: "Iodine-129", Element: "I", MassNumber: 129, HalfLife: "1.57e7 y", Category: "fission"},
	{NuclideID: "I-131", DisplayName: "Iodine-131", Element: "I", MassNumber: 131, HalfLife: "8.02 d", Category: "fission", GammaLinesKeV: []float64{364.5, 637.0}},
	{NuclideID: "Cs-134", DisplayName: "Cesium-134", Element: "Cs", MassNumber: 134, HalfLife: "2.06 y", Category: "reactor", GammaLinesKeV: []float64{569.3, 604.7, 795.8, 801.9}},
	{NuclideID: "Cs-136", DisplayName: "Cesium-136", Element: "Cs", MassNumber: 136, HalfLife: "13.2 d", Category: "reactor", GammaLinesKeV: []float64{818.5, 1048.1}},
	{NuclideID: "Cs-137", DisplayName: "Cesium-137", Element: "Cs", MassNumber: 137, HalfLife: "30.1 y", Category: "fallout", BetaMaxKeV: []float64{1176.0, 514.0}, GammaLinesKeV: []float64{661.7}},
	{NuclideID: "Ba-133", DisplayName: "Barium-133", Element: "Ba", MassNumber: 133, HalfLife: "10.5 y", Category: "calibration", GammaLinesKeV: []float64{81.0, 356.0, 383.8}},
	{NuclideID: "Ba-140", DisplayName: "Barium-140", Element: "Ba", MassNumber: 140, HalfLife: "12.8 d", Category: "fission", GammaLinesKeV: []float64{537.3}},
	{NuclideID: "La-140", DisplayName: "Lanthanum-140", Element: "La", MassNumber: 140, HalfLife: "1.68 d", Category: "fission", GammaLinesKeV: []float64{487.0, 1596.2}},
	{NuclideID: "Ce-141", DisplayName: "Cerium-141", Element: "Ce", MassNumber: 141, HalfLife: "32.5 d", Category: "fission", GammaLinesKeV: []float64{145.4}},
	{NuclideID: "Ce-144", DisplayName: "Cerium-144", Element: "Ce", MassNumber: 144, HalfLife: "284.9 d", Category: "fission"},
	{NuclideID: "Pr-144", DisplayName: "Praseodymium-144", Element: "Pr", MassNumber: 144, HalfLife: "17.3 min", Category: "fission", GammaLinesKeV: []float64{696.5, 1489.0}},
	{NuclideID: "Eu-152", DisplayName: "Europium-152", Element: "Eu", MassNumber: 152, HalfLife: "13.5 y", Category: "calibration", GammaLinesKeV: []float64{121.8, 244.7, 344.3, 778.9, 964.1, 1408.0}},
	{NuclideID: "Eu-154", DisplayName: "Europium-154", Element: "Eu", MassNumber: 154, HalfLife: "8.6 y", Category: "fission", GammaLinesKeV: []float64{723.3, 873.2, 1004.7, 1274.4}},
	{NuclideID: "Eu-155", DisplayName: "Europium-155", Element: "Eu", MassNumber: 155, HalfLife: "4.76 y", Category: "fission", GammaLinesKeV: []float64{86.5, 105.3}},
	{NuclideID: "Gd-153", DisplayName: "Gadolinium-153", Element: "Gd", MassNumber: 153, HalfLife: "240 d", Category: "calibration", GammaLinesKeV: []float64{97.4, 103.2}},
	{NuclideID: "Tb-160", DisplayName: "Terbium-160", Element: "Tb", MassNumber: 160, HalfLife: "72.3 d", Category: "activation", GammaLinesKeV: []float64{879.4, 966.2}},
	{NuclideID: "Ir-192", DisplayName: "Iridium-192", Element: "Ir", MassNumber: 192, HalfLife: "73.8 d", Category: "industrial", GammaLinesKeV: []float64{295.9, 308.5, 316.5, 468.1, 604.4}},
	{NuclideID: "Au-198", DisplayName: "Gold-198", Element: "Au", MassNumber: 198, HalfLife: "2.69 d", Category: "activation", GammaLinesKeV: []float64{411.8}},
	{NuclideID: "Hg-203", DisplayName: "Mercury-203", Element: "Hg", MassNumber: 203, HalfLife: "46.6 d", Category: "activation", GammaLinesKeV: []float64{279.2}},
	{NuclideID: "Tl-201", DisplayName: "Thallium-201", Element: "Tl", MassNumber: 201, HalfLife: "73 h", Category: "medical", GammaLinesKeV: []float64{135.3, 167.4}},
	{NuclideID: "Pb-203", DisplayName: "Lead-203", Element: "Pb", MassNumber: 203, HalfLife: "51.9 h", Category: "medical", GammaLinesKeV: []float64{279.2}},
	{NuclideID: "Po-210", DisplayName: "Polonium-210", Element: "Po", MassNumber: 210, HalfLife: "138.4 d", Category: "NORM"},
	{NuclideID: "Ra-226", DisplayName: "Radium-226", Element: "Ra", MassNumber: 226, HalfLife: "1600 y", Category: "NORM", GammaLinesKeV: []float64{186.2}},
	{NuclideID: "Ra-228", DisplayName: "Radium-228", Element: "Ra", MassNumber: 228, HalfLife: "5.75 y", Category: "NORM"},
	{NuclideID: "Am-241", DisplayName: "Americium-241", Element: "Am", MassNumber: 241, HalfLife: "432.2 y", Category: "industrial", TypicalSource: "smoke detectors", AlphaLinesKeV: []float64{5485.6}, GammaLinesKeV: []float64{59.5}},
	{NuclideID: "Am-243", DisplayName: "Americium-243", Element: "Am", MassNumber: 243, HalfLife: "7370 y", Category: "actinide"},
	{NuclideID: "Cm-242", DisplayName: "Curium-242", Element: "Cm", MassNumber: 242, HalfLife: "163 d", Category: "actinide"},
	{NuclideID: "Cm-244", DisplayName: "Curium-244", Element: "Cm", MassNumber: 244, HalfLife: "18.1 y", Category: "actinide"},
	{NuclideID: "Pu-238", DisplayName: "Plutonium-238", Element: "Pu", MassNumber: 238, HalfLife: "87.7 y", Category: "actinide"},
	{NuclideID: "Pu-239", DisplayName: "Plutonium-239", Element: "Pu", MassNumber: 239, HalfLife: "24110 y", Category: "actinide", GammaLinesKeV: []float64{129.3}},
	{NuclideID: "Pu-240", DisplayName: "Plutonium-240", Element: "Pu", MassNumber: 240, HalfLife: "6561 y", Category: "actinide"},
	{NuclideID: "Pu-241", DisplayName: "Plutonium-241", Element: "Pu", MassNumber: 241, HalfLife: "14.3 y", Category: "actinide"},
	{NuclideID: "Pu-242", DisplayName: "Plutonium-242", Element: "Pu", MassNumber: 242, HalfLife: "3.75e5 y", Category: "actinide"},
	{NuclideID: "Np-239", DisplayName: "Neptunium-239", Element: "Np", MassNumber: 239, HalfLife: "2.36 d", Category: "reactor", GammaLinesKeV: []float64{106.1, 228.2}},
	{NuclideID: "Cf-252", DisplayName: "Californium-252", Element: "Cf", MassNumber: 252, HalfLife: "2.65 y", Category: "neutron source"},

	// Extended practical isotope set used by import validation and modal explanations.
	{NuclideID: "Ag-108m", DisplayName: "Silver-108m", Element: "Ag", MassNumber: 108, HalfLife: "438 y", Category: "activation", TypicalSource: "silver activation in electronics", GammaLinesKeV: []float64{433.9, 614.3, 722.9}},
	{NuclideID: "Al-26", DisplayName: "Aluminum-26", Element: "Al", MassNumber: 26, HalfLife: "7.17e5 y", Category: "cosmogenic", TypicalSource: "meteorites and cosmogenic dust", GammaLinesKeV: []float64{511.0, 1808.7}},
	{NuclideID: "Ar-37", DisplayName: "Argon-37", Element: "Ar", MassNumber: 37, HalfLife: "35 d", Category: "activation", TypicalSource: "subsurface activation and nuclear processes", GammaLinesKeV: []float64{2.8}},
	{NuclideID: "Ar-39", DisplayName: "Argon-39", Element: "Ar", MassNumber: 39, HalfLife: "269 y", Category: "cosmogenic", TypicalSource: "atmospheric argon", BetaMaxKeV: []float64{565.0}},
	{NuclideID: "Ar-41", DisplayName: "Argon-41", Element: "Ar", MassNumber: 41, HalfLife: "1.83 h", Category: "activation", TypicalSource: "air activation near reactors", GammaLinesKeV: []float64{1293.6}},
	{NuclideID: "As-72", DisplayName: "Arsenic-72", Element: "As", MassNumber: 72, HalfLife: "26 h", Category: "medical", TypicalSource: "activation and radiotracer studies", GammaLinesKeV: []float64{511.0, 834.0, 1464.0}},
	{NuclideID: "As-74", DisplayName: "Arsenic-74", Element: "As", MassNumber: 74, HalfLife: "17.8 d", Category: "activation", TypicalSource: "activation products in materials", GammaLinesKeV: []float64{596.0, 635.0, 1204.0}},
	{NuclideID: "As-76", DisplayName: "Arsenic-76", Element: "As", MassNumber: 76, HalfLife: "26.3 h", Category: "activation", TypicalSource: "arsenic activation in ores", GammaLinesKeV: []float64{559.0, 657.0, 1216.0}},
	{NuclideID: "Be-10", DisplayName: "Beryllium-10", Element: "Be", MassNumber: 10, HalfLife: "1.39e6 y", Category: "cosmogenic", TypicalSource: "ice cores and atmospheric deposition", BetaMaxKeV: []float64{556.0}},
	{NuclideID: "Bi-207", DisplayName: "Bismuth-207", Element: "Bi", MassNumber: 207, HalfLife: "31.6 y", Category: "calibration", TypicalSource: "calibration and activation source", GammaLinesKeV: []float64{570.0, 1064.0, 1770.0}},
	{NuclideID: "Br-82", DisplayName: "Bromine-82", Element: "Br", MassNumber: 82, HalfLife: "35.3 h", Category: "activation", TypicalSource: "activated bromine in salts and water", GammaLinesKeV: []float64{554.0, 619.0, 776.0, 1044.0, 1317.0}},
	{NuclideID: "Ca-45", DisplayName: "Calcium-45", Element: "Ca", MassNumber: 45, HalfLife: "162.7 d", Category: "medical", TypicalSource: "biomedical calcium tracing", BetaMaxKeV: []float64{257.0}},
	{NuclideID: "Ca-47", DisplayName: "Calcium-47", Element: "Ca", MassNumber: 47, HalfLife: "4.54 d", Category: "medical", TypicalSource: "reactor-produced medical tracer", GammaLinesKeV: []float64{489.0, 1297.0}},
	{NuclideID: "Cd-115m", DisplayName: "Cadmium-115m", Element: "Cd", MassNumber: 115, HalfLife: "44.6 d", Category: "fission", TypicalSource: "fission and cadmium activation", GammaLinesKeV: []float64{336.0}},
	{NuclideID: "Ce-139", DisplayName: "Cerium-139", Element: "Ce", MassNumber: 139, HalfLife: "137.6 d", Category: "calibration", TypicalSource: "calibration and activation sources", GammaLinesKeV: []float64{165.9}},
	{NuclideID: "Cl-36", DisplayName: "Chlorine-36", Element: "Cl", MassNumber: 36, HalfLife: "3.01e5 y", Category: "cosmogenic", TypicalSource: "cosmogenic chlorine in water and salts", BetaMaxKeV: []float64{709.0}},
	{NuclideID: "Co-56", DisplayName: "Cobalt-56", Element: "Co", MassNumber: 56, HalfLife: "77.2 d", Category: "activation", TypicalSource: "steel activation and astrophysical studies", GammaLinesKeV: []float64{511.0, 846.8, 1037.8, 1238.0, 1771.0}},
	{NuclideID: "Cr-51", DisplayName: "Chromium-51", Element: "Cr", MassNumber: 51, HalfLife: "27.7 d", Category: "activation", TypicalSource: "stainless steel activation", GammaLinesKeV: []float64{320.1}},
	{NuclideID: "Cs-135", DisplayName: "Cesium-135", Element: "Cs", MassNumber: 135, HalfLife: "2.3e6 y", Category: "fission", TypicalSource: "long-lived fission product in waste", BetaMaxKeV: []float64{269.0}},
	{NuclideID: "Cu-64", DisplayName: "Copper-64", Element: "Cu", MassNumber: 64, HalfLife: "12.7 h", Category: "medical", TypicalSource: "PET theranostics", GammaLinesKeV: []float64{511.0, 1345.8}},
	{NuclideID: "Cu-67", DisplayName: "Copper-67", Element: "Cu", MassNumber: 67, HalfLife: "61.8 h", Category: "medical", TypicalSource: "targeted radionuclide therapy", GammaLinesKeV: []float64{93.0, 185.0, 300.0}},
	{NuclideID: "Dy-165", DisplayName: "Dysprosium-165", Element: "Dy", MassNumber: 165, HalfLife: "2.33 h", Category: "activation", TypicalSource: "rare-earth activation products", GammaLinesKeV: []float64{95.0}},
	{NuclideID: "Er-169", DisplayName: "Erbium-169", Element: "Er", MassNumber: 169, HalfLife: "9.4 d", Category: "medical", TypicalSource: "joint therapy radiopharmaceuticals", BetaMaxKeV: []float64{351.0}},
	{NuclideID: "Fe-55", DisplayName: "Iron-55", Element: "Fe", MassNumber: 55, HalfLife: "2.74 y", Category: "activation", TypicalSource: "iron activation in metals", GammaLinesKeV: []float64{5.9}},
	{NuclideID: "Fe-59", DisplayName: "Iron-59", Element: "Fe", MassNumber: 59, HalfLife: "44.5 d", Category: "activation", TypicalSource: "steel activation and biomedicine", GammaLinesKeV: []float64{1099.3, 1291.6}},
	{NuclideID: "Ga-67", DisplayName: "Gallium-67", Element: "Ga", MassNumber: 67, HalfLife: "78.3 h", Category: "medical", TypicalSource: "nuclear medicine diagnostics", GammaLinesKeV: []float64{93.3, 184.6, 300.2, 393.5}},
	{NuclideID: "Ga-68", DisplayName: "Gallium-68", Element: "Ga", MassNumber: 68, HalfLife: "67.7 min", Category: "medical", TypicalSource: "Ge-68/Ga-68 PET generators", GammaLinesKeV: []float64{511.0, 1077.0}},
	{NuclideID: "Ge-68", DisplayName: "Germanium-68", Element: "Ge", MassNumber: 68, HalfLife: "271 d", Category: "medical", TypicalSource: "PET generator parent isotope", GammaLinesKeV: []float64{511.0}},
	{NuclideID: "Hf-181", DisplayName: "Hafnium-181", Element: "Hf", MassNumber: 181, HalfLife: "42.4 d", Category: "activation", TypicalSource: "hafnium activation in alloys", GammaLinesKeV: []float64{133.0, 345.0, 482.0}},
	{NuclideID: "Ho-166", DisplayName: "Holmium-166", Element: "Ho", MassNumber: 166, HalfLife: "26.8 h", Category: "medical", TypicalSource: "liver and tumor microsphere therapy", GammaLinesKeV: []float64{80.6}},
	{NuclideID: "I-132", DisplayName: "Iodine-132", Element: "I", MassNumber: 132, HalfLife: "2.3 h", Category: "fission", TypicalSource: "fresh fallout from Te-132 decay", GammaLinesKeV: []float64{667.7, 772.6, 954.6}},
	{NuclideID: "I-133", DisplayName: "Iodine-133", Element: "I", MassNumber: 133, HalfLife: "20.8 h", Category: "fission", TypicalSource: "fresh fission product", GammaLinesKeV: []float64{529.9, 875.3}},
	{NuclideID: "I-135", DisplayName: "Iodine-135", Element: "I", MassNumber: 135, HalfLife: "6.6 h", Category: "fission", TypicalSource: "short-lived reactor fission product", GammaLinesKeV: []float64{1260.0, 1678.0}},
	{NuclideID: "In-111", DisplayName: "Indium-111", Element: "In", MassNumber: 111, HalfLife: "2.8 d", Category: "medical", TypicalSource: "labeled antibodies and leukocytes", GammaLinesKeV: []float64{171.3, 245.4}},
	{NuclideID: "In-113m", DisplayName: "Indium-113m", Element: "In", MassNumber: 113, HalfLife: "99.5 min", Category: "medical", TypicalSource: "generator-based medical isotope", GammaLinesKeV: []float64{391.7}},
	{NuclideID: "K-42", DisplayName: "Potassium-42", Element: "K", MassNumber: 42, HalfLife: "12.4 h", Category: "activation", TypicalSource: "potassium activation in reactor systems", GammaLinesKeV: []float64{1524.7}},
	{NuclideID: "Lu-177", DisplayName: "Lutetium-177", Element: "Lu", MassNumber: 177, HalfLife: "6.65 d", Category: "medical", TypicalSource: "targeted radionuclide therapy", GammaLinesKeV: []float64{113.0, 208.0}},
	{NuclideID: "Mg-28", DisplayName: "Magnesium-28", Element: "Mg", MassNumber: 28, HalfLife: "20.9 h", Category: "activation", TypicalSource: "activation and fission products", GammaLinesKeV: []float64{30.0, 400.0, 941.0}},
	{NuclideID: "Mn-52", DisplayName: "Manganese-52", Element: "Mn", MassNumber: 52, HalfLife: "5.59 d", Category: "medical", TypicalSource: "activation and PET research", GammaLinesKeV: []float64{511.0, 744.0, 935.0, 1434.0}},
	{NuclideID: "Mn-54", DisplayName: "Manganese-54", Element: "Mn", MassNumber: 54, HalfLife: "312 d", Category: "activation", TypicalSource: "steel activation", GammaLinesKeV: []float64{834.8}},
	{NuclideID: "Mn-56", DisplayName: "Manganese-56", Element: "Mn", MassNumber: 56, HalfLife: "2.58 h", Category: "activation", TypicalSource: "activation in metals", GammaLinesKeV: []float64{846.8, 1810.7, 2113.0}},
	{NuclideID: "Nb-94", DisplayName: "Niobium-94", Element: "Nb", MassNumber: 94, HalfLife: "2.03e4 y", Category: "activation", TypicalSource: "niobium activation in reactor materials", GammaLinesKeV: []float64{702.6, 871.1}},
	{NuclideID: "Nd-147", DisplayName: "Neodymium-147", Element: "Nd", MassNumber: 147, HalfLife: "11 d", Category: "fission", TypicalSource: "fission product in waste", GammaLinesKeV: []float64{91.0, 531.0}},
	{NuclideID: "Ni-59", DisplayName: "Nickel-59", Element: "Ni", MassNumber: 59, HalfLife: "7.6e4 y", Category: "activation", TypicalSource: "nickel activation in steel", GammaLinesKeV: []float64{7.0}},
	{NuclideID: "Ni-63", DisplayName: "Nickel-63", Element: "Ni", MassNumber: 63, HalfLife: "100.1 y", Category: "activation", TypicalSource: "nickel activation and sensor sources", BetaMaxKeV: []float64{66.9}},
	{NuclideID: "P-32", DisplayName: "Phosphorus-32", Element: "P", MassNumber: 32, HalfLife: "14.3 d", Category: "medical", TypicalSource: "biological tracing and therapy", BetaMaxKeV: []float64{1710.0}},
	{NuclideID: "P-33", DisplayName: "Phosphorus-33", Element: "P", MassNumber: 33, HalfLife: "25.3 d", Category: "medical", TypicalSource: "laboratory tracer", BetaMaxKeV: []float64{249.0}},
	{NuclideID: "Pd-103", DisplayName: "Palladium-103", Element: "Pd", MassNumber: 103, HalfLife: "17 d", Category: "medical", TypicalSource: "brachytherapy seeds", GammaLinesKeV: []float64{21.0}},
	{NuclideID: "Pm-147", DisplayName: "Promethium-147", Element: "Pm", MassNumber: 147, HalfLife: "2.62 y", Category: "industrial", TypicalSource: "fission product and industrial sources", BetaMaxKeV: []float64{225.0}},
	{NuclideID: "Pm-148m", DisplayName: "Promethium-148m", Element: "Pm", MassNumber: 148, HalfLife: "41.3 d", Category: "fission", TypicalSource: "fission and activation products", GammaLinesKeV: []float64{550.0, 629.0, 725.0, 915.0, 1014.0}},
	{NuclideID: "Rb-86", DisplayName: "Rubidium-86", Element: "Rb", MassNumber: 86, HalfLife: "18.6 d", Category: "activation", TypicalSource: "activation and laboratory tracers", GammaLinesKeV: []float64{1076.6}},
	{NuclideID: "Re-186", DisplayName: "Rhenium-186", Element: "Re", MassNumber: 186, HalfLife: "3.72 d", Category: "medical", TypicalSource: "targeted therapy", GammaLinesKeV: []float64{137.0}},
	{NuclideID: "Re-188", DisplayName: "Rhenium-188", Element: "Re", MassNumber: 188, HalfLife: "17 h", Category: "medical", TypicalSource: "generator-based therapeutic isotope", GammaLinesKeV: []float64{155.0}},
	{NuclideID: "Rh-106", DisplayName: "Rhodium-106", Element: "Rh", MassNumber: 106, HalfLife: "30 s", Category: "fission", TypicalSource: "daughter of Ru-106", GammaLinesKeV: []float64{511.0, 622.0, 1050.0}},
	{NuclideID: "S-35", DisplayName: "Sulfur-35", Element: "S", MassNumber: 35, HalfLife: "87.5 d", Category: "medical", TypicalSource: "biological tracer", BetaMaxKeV: []float64{167.0}},
	{NuclideID: "Sb-122", DisplayName: "Antimony-122", Element: "Sb", MassNumber: 122, HalfLife: "2.72 d", Category: "activation", TypicalSource: "activated antimony in alloys", GammaLinesKeV: []float64{564.0, 693.0}},
	{NuclideID: "Sc-46", DisplayName: "Scandium-46", Element: "Sc", MassNumber: 46, HalfLife: "83.8 d", Category: "activation", TypicalSource: "activation of scandium/titanium alloys", GammaLinesKeV: []float64{889.3, 1120.5}},
	{NuclideID: "Sc-47", DisplayName: "Scandium-47", Element: "Sc", MassNumber: 47, HalfLife: "3.35 d", Category: "medical", TypicalSource: "theranostic isotope production", GammaLinesKeV: []float64{159.4}},
	{NuclideID: "Sc-48", DisplayName: "Scandium-48", Element: "Sc", MassNumber: 48, HalfLife: "43.7 h", Category: "activation", TypicalSource: "activation products", GammaLinesKeV: []float64{984.0, 1037.0, 1312.0}},
	{NuclideID: "Si-31", DisplayName: "Silicon-31", Element: "Si", MassNumber: 31, HalfLife: "2.62 h", Category: "activation", TypicalSource: "silicon activation in sand and electronics", BetaMaxKeV: []float64{1490.0}},
	{NuclideID: "Sm-151", DisplayName: "Samarium-151", Element: "Sm", MassNumber: 151, HalfLife: "90 y", Category: "fission", TypicalSource: "long-lived fission product", BetaMaxKeV: []float64{77.0}},
	{NuclideID: "Sm-153", DisplayName: "Samarium-153", Element: "Sm", MassNumber: 153, HalfLife: "46.3 h", Category: "medical", TypicalSource: "bone pain palliation therapy", GammaLinesKeV: []float64{103.2}},
	{NuclideID: "Sn-113", DisplayName: "Tin-113", Element: "Sn", MassNumber: 113, HalfLife: "115 d", Category: "calibration", TypicalSource: "source for In-113m generators", GammaLinesKeV: []float64{391.7}},
	{NuclideID: "Sn-117m", DisplayName: "Tin-117m", Element: "Sn", MassNumber: 117, HalfLife: "13.6 d", Category: "medical", TypicalSource: "therapeutic isotope for bone disease", GammaLinesKeV: []float64{158.6}},
	{NuclideID: "Sn-125", DisplayName: "Tin-125", Element: "Sn", MassNumber: 125, HalfLife: "9.64 d", Category: "fission", TypicalSource: "fission product in waste", GammaLinesKeV: []float64{822.0, 1067.0}},
	{NuclideID: "Sr-85", DisplayName: "Strontium-85", Element: "Sr", MassNumber: 85, HalfLife: "64.8 d", Category: "medical", TypicalSource: "bone metabolism tracer", GammaLinesKeV: []float64{514.0}},
	{NuclideID: "Ta-182", DisplayName: "Tantalum-182", Element: "Ta", MassNumber: 182, HalfLife: "114.4 d", Category: "activation", TypicalSource: "activation in tantalum components", GammaLinesKeV: []float64{67.8, 100.0, 222.0, 1121.0, 1221.0}},
	{NuclideID: "Tc-99", DisplayName: "Technetium-99", Element: "Tc", MassNumber: 99, HalfLife: "2.11e5 y", Category: "fission", TypicalSource: "long-lived fission product", BetaMaxKeV: []float64{294.0}},
	{NuclideID: "Te-125m", DisplayName: "Tellurium-125m", Element: "Te", MassNumber: 125, HalfLife: "57.4 d", Category: "fission", TypicalSource: "fission product", GammaLinesKeV: []float64{35.5, 109.0, 145.0}},
	{NuclideID: "Te-127m", DisplayName: "Tellurium-127m", Element: "Te", MassNumber: 127, HalfLife: "107 d", Category: "fission", TypicalSource: "fission product", GammaLinesKeV: []float64{88.0, 417.0}},
	{NuclideID: "Te-129m", DisplayName: "Tellurium-129m", Element: "Te", MassNumber: 129, HalfLife: "33.6 d", Category: "fission", TypicalSource: "fission product", GammaLinesKeV: []float64{696.0}},
	{NuclideID: "Te-132", DisplayName: "Tellurium-132", Element: "Te", MassNumber: 132, HalfLife: "3.2 d", Category: "fission", TypicalSource: "short-lived fresh fallout", GammaLinesKeV: []float64{228.0, 773.0}},
	{NuclideID: "Ti-44", DisplayName: "Titanium-44", Element: "Ti", MassNumber: 44, HalfLife: "59 y", Category: "cosmogenic", TypicalSource: "supernova and meteoritic material", GammaLinesKeV: []float64{67.9, 78.3, 1157.0}},
	{NuclideID: "Tm-170", DisplayName: "Thulium-170", Element: "Tm", MassNumber: 170, HalfLife: "128.6 d", Category: "industrial", TypicalSource: "industrial low-energy source", GammaLinesKeV: []float64{52.0, 84.0}},
	{NuclideID: "U-232", DisplayName: "Uranium-232", Element: "U", MassNumber: 232, HalfLife: "68.9 y", Category: "actinide", TypicalSource: "thorium cycle and reprocessed fuel"},
	{NuclideID: "V-48", DisplayName: "Vanadium-48", Element: "V", MassNumber: 48, HalfLife: "16 d", Category: "activation", TypicalSource: "activation in titanium and vanadium alloys", GammaLinesKeV: []float64{511.0, 983.5, 1312.0}},
	{NuclideID: "W-181", DisplayName: "Tungsten-181", Element: "W", MassNumber: 181, HalfLife: "121 d", Category: "activation", TypicalSource: "tungsten activation in alloys", GammaLinesKeV: []float64{136.0}},
	{NuclideID: "W-185", DisplayName: "Tungsten-185", Element: "W", MassNumber: 185, HalfLife: "75.1 d", Category: "activation", TypicalSource: "tungsten activation", GammaLinesKeV: []float64{125.0}},
	{NuclideID: "W-187", DisplayName: "Tungsten-187", Element: "W", MassNumber: 187, HalfLife: "23.7 h", Category: "activation", TypicalSource: "tungsten activation", GammaLinesKeV: []float64{479.0, 686.0}},
	{NuclideID: "Xe-131m", DisplayName: "Xenon-131m", Element: "Xe", MassNumber: 131, HalfLife: "11.8 d", Category: "fission", TypicalSource: "fission and medical gas sources", GammaLinesKeV: []float64{164.0}},
	{NuclideID: "Xe-133", DisplayName: "Xenon-133", Element: "Xe", MassNumber: 133, HalfLife: "5.25 d", Category: "fission", TypicalSource: "reactor releases and lung imaging", GammaLinesKeV: []float64{81.0}},
	{NuclideID: "Xe-135", DisplayName: "Xenon-135", Element: "Xe", MassNumber: 135, HalfLife: "9.14 h", Category: "fission", TypicalSource: "reactor poison from fission", GammaLinesKeV: []float64{249.8}},
	{NuclideID: "Y-88", DisplayName: "Yttrium-88", Element: "Y", MassNumber: 88, HalfLife: "106.6 d", Category: "calibration", TypicalSource: "activation and calibration", GammaLinesKeV: []float64{511.0, 898.0, 1836.1}},
	{NuclideID: "Yb-169", DisplayName: "Ytterbium-169", Element: "Yb", MassNumber: 169, HalfLife: "32 d", Category: "industrial", TypicalSource: "industrial radiography source", GammaLinesKeV: []float64{63.0, 109.0, 130.0, 177.0, 198.0, 308.0}},
	{NuclideID: "Zr-89", DisplayName: "Zirconium-89", Element: "Zr", MassNumber: 89, HalfLife: "78.4 h", Category: "medical", TypicalSource: "immuno-PET tracers", GammaLinesKeV: []float64{511.0, 909.0}},

	// Heavy transuranics for completeness envelope.
	{NuclideID: "Bk-249", DisplayName: "Berkelium-249", Element: "Bk", MassNumber: 249, HalfLife: "330 d", Category: "transuranic"},
	{NuclideID: "Es-254", DisplayName: "Einsteinium-254", Element: "Es", MassNumber: 254, HalfLife: "276 d", Category: "transuranic"},
	{NuclideID: "Fm-257", DisplayName: "Fermium-257", Element: "Fm", MassNumber: 257, HalfLife: "100 d", Category: "transuranic"},
	{NuclideID: "Md-258", DisplayName: "Mendelevium-258", Element: "Md", MassNumber: 258, HalfLife: "51.5 d", Category: "transuranic"},
	{NuclideID: "No-259", DisplayName: "Nobelium-259", Element: "No", MassNumber: 259, HalfLife: "58 min", Category: "transuranic"},
	{NuclideID: "Lr-262", DisplayName: "Lawrencium-262", Element: "Lr", MassNumber: 262, HalfLife: "3.6 h", Category: "transuranic"},
	{NuclideID: "Rf-267", DisplayName: "Rutherfordium-267", Element: "Rf", MassNumber: 267, HalfLife: "1.3 h", Category: "superheavy"},
	{NuclideID: "Db-268", DisplayName: "Dubnium-268", Element: "Db", MassNumber: 268, HalfLife: "29 h", Category: "superheavy"},
	{NuclideID: "Sg-271", DisplayName: "Seaborgium-271", Element: "Sg", MassNumber: 271, HalfLife: "2.4 min", Category: "superheavy"},
	{NuclideID: "Bh-270", DisplayName: "Bohrium-270", Element: "Bh", MassNumber: 270, HalfLife: "61 s", Category: "superheavy"},
	{NuclideID: "Hs-277", DisplayName: "Hassium-277", Element: "Hs", MassNumber: 277, HalfLife: "11 min", Category: "superheavy"},
	{NuclideID: "Mt-278", DisplayName: "Meitnerium-278", Element: "Mt", MassNumber: 278, HalfLife: "8 s", Category: "superheavy"},
	{NuclideID: "Ds-281", DisplayName: "Darmstadtium-281", Element: "Ds", MassNumber: 281, HalfLife: "10 s", Category: "superheavy"},
	{NuclideID: "Rg-282", DisplayName: "Roentgenium-282", Element: "Rg", MassNumber: 282, HalfLife: "2.1 min", Category: "superheavy"},
	{NuclideID: "Cn-285", DisplayName: "Copernicium-285", Element: "Cn", MassNumber: 285, HalfLife: "29 s", Category: "superheavy"},
	{NuclideID: "Nh-286", DisplayName: "Nihonium-286", Element: "Nh", MassNumber: 286, HalfLife: "20 s", Category: "superheavy"},
	{NuclideID: "Fl-289", DisplayName: "Flerovium-289", Element: "Fl", MassNumber: 289, HalfLife: "2.6 s", Category: "superheavy"},
	{NuclideID: "Mc-290", DisplayName: "Moscovium-290", Element: "Mc", MassNumber: 290, HalfLife: "0.65 s", Category: "superheavy"},
	{NuclideID: "Lv-293", DisplayName: "Livermorium-293", Element: "Lv", MassNumber: 293, HalfLife: "61 ms", Category: "superheavy"},
	{NuclideID: "Ts-294", DisplayName: "Tennessine-294", Element: "Ts", MassNumber: 294, HalfLife: "21 ms", Category: "superheavy"},
	{NuclideID: "Og-294", DisplayName: "Oganesson-294", Element: "Og", MassNumber: 294, HalfLife: "0.69 ms", Category: "superheavy"},
}
