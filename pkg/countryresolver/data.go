package countryresolver

import "sort"

const (
	priorityEnclave   = 0  // Highest precedence to avoid larger neighbours swallowing enclaves.
	priorityCityState = 5  // City states need to win over their surrounding countries.
	prioritySmall     = 10 // Small border countries should beat regional rectangles.
	priorityRegional  = 20 // Normal precedence for mid-sized countries.
	priorityLarge     = 40 // Large countries that can safely come later.
	priorityHuge      = 80 // Massive rectangles only act as fallbacks.
)

// buildBoxes enumerates coarse rectangular boundaries for countries
// where realtime sensors have been observed.  The list intentionally
// prefers more precise rectangles so sensors near borders resolve to the
// expected state.
func buildBoxes() []regionBox {
	regions := []regionBox{
		// Special administrative regions and city states.
		newBox(priorityEnclave, "MO", "Macao", 22.0, 22.4, 113.4, 113.7),
		newBox(priorityEnclave, "HK", "Hong Kong", 22.1, 22.6, 113.8, 114.5),
		newBox(priorityCityState, "SG", "Singapore", 1.1, 1.6, 103.6, 104.1),

		// Africa.
		newBox(prioritySmall, "RW", "Rwanda", -3.0, -0.5, 28.5, 31.0),
		newBox(priorityRegional, "SN", "Senegal", 12.0, 17.5, -18.5, -11.0),
		newBox(priorityRegional, "CI", "Côte d'Ivoire", 4.0, 10.0, -8.6, -2.5),
		newBox(priorityRegional, "GH", "Ghana", 4.0, 11.5, -3.5, 1.8),
		newBox(prioritySmall, "TG", "Togo", 6.0, 11.5, -0.5, 1.8),
		newBox(prioritySmall, "BJ", "Benin", 6.0, 12.5, 1.0, 3.8),
		newBox(priorityRegional, "NG", "Nigeria", 4.0, 14.5, 2.0, 15.0),
		newBox(priorityRegional, "ZA", "South Africa", -35.0, -21.0, 16.0, 33.0),

		// South and Southeast Asia.
		newBox(priorityRegional, "TH", "Thailand", 5.0, 21.0, 97.0, 106.0),
		newBox(priorityRegional, "VN", "Vietnam", 8.0, 24.5, 102.0, 110.5),
		newBox(priorityLarge, "IN", "India", 6.0, 37.5, 68.0, 98.0),
		newBox(prioritySmall, "TW", "Taiwan", 21.0, 26.5, 119.0, 123.5),
		newBox(prioritySmall, "KR", "South Korea", 33.0, 39.5, 124.0, 131.5),
		newBox(priorityRegional, "JP", "Japan", 30.0, 46.5, 129.0, 146.5),
		newBox(priorityRegional, "JP", "Japan", 24.0, 29.5, 122.0, 132.0),
		newBox(priorityHuge, "CN", "China", 18.0, 54.0, 73.0, 135.0),

		// Oceania.
		newBox(priorityRegional, "US", "United States", 24.0, 49.5, -125.0, -66.0),
		newBox(priorityRegional, "US", "United States", 18.0, 23.0, -161.0, -154.0),
		newBox(priorityRegional, "US", "United States", 51.0, 72.0, -171.0, -129.0),
		newBox(priorityHuge, "CA", "Canada", 41.0, 84.0, -141.0, -52.0),
		newBox(priorityRegional, "MX", "Mexico", 14.0, 33.0, -118.0, -86.0),
		newBox(priorityRegional, "NZ", "New Zealand", -48.0, -33.0, 165.0, 180.0),
		newBox(priorityLarge, "AU", "Australia", -45.0, -9.0, 112.0, 155.0),
		newBox(prioritySmall, "FJ", "Fiji", -21.5, -15.0, 176.0, 180.0),

		// South America.
		newBox(priorityRegional, "BO", "Bolivia", -23.0, -9.0, -70.0, -57.0),
		newBox(priorityRegional, "PE", "Peru", -19.0, -3.0, -82.0, -68.0),
		newBox(priorityLarge, "BR", "Brazil", -35.0, 6.0, -75.0, -34.0),
		newBox(priorityRegional, "AR", "Argentina", -56.0, -21.0, -74.0, -53.0),
		newBox(priorityRegional, "CL", "Chile", -56.0, -17.0, -76.0, -66.0),
		newBox(priorityRegional, "CO", "Colombia", -5.0, 13.0, -79.0, -66.0),
		newBox(priorityRegional, "EC", "Ecuador", -5.5, 2.0, -92.0, -75.0),
		newBox(priorityRegional, "UY", "Uruguay", -35.0, -30.0, -58.5, -53.0),
		newBox(priorityRegional, "PY", "Paraguay", -28.0, -18.0, -63.5, -54.0),

		// Western Europe.
		newBox(priorityRegional, "ES", "Spain", 35.5, 44.5, -10.0, 4.5),
		newBox(priorityRegional, "PT", "Portugal", 36.5, 42.5, -9.8, -6.0),
		newBox(priorityRegional, "FR", "France", 41.0, 51.5, -5.5, 9.5),
		newBox(priorityRegional, "IT", "Italy", 36.0, 47.5, 6.0, 19.0),
		newBox(prioritySmall, "CH", "Switzerland", 45.5, 48.5, 5.0, 11.0),
		newBox(prioritySmall, "SI", "Slovenia", 45.3, 47.0, 13.0, 16.0),
		newBox(prioritySmall, "HR", "Croatia", 42.0, 47.0, 13.0, 20.5),
		newBox(priorityRegional, "AT", "Austria", 46.0, 49.5, 9.0, 17.0),
		newBox(priorityRegional, "DE", "Germany", 47.0, 55.5, 5.0, 16.0),
		newBox(priorityRegional, "NL", "Netherlands", 50.5, 54.5, 3.0, 8.0),
		newBox(prioritySmall, "BE", "Belgium", 49.5, 51.7, 2.0, 6.6),
		newBox(prioritySmall, "LU", "Luxembourg", 49.3, 51.0, 5.5, 6.6),
		newBox(priorityRegional, "UK", "United Kingdom", 49.5, 60.0, -9.5, 2.5),
		newBox(priorityRegional, "IE", "Ireland", 51.0, 56.0, -11.0, -5.0),
		newBox(priorityRegional, "IS", "Iceland", 63.0, 67.5, -25.0, -12.0),
		newBox(priorityRegional, "NO", "Norway", 58.0, 72.5, 5.0, 32.5),
		newBox(priorityRegional, "NO", "Norway", 74.0, 82.5, 5.0, 34.0),
		newBox(priorityRegional, "SE", "Sweden", 55.0, 70.5, 11.0, 25.5),
		newBox(priorityRegional, "FI", "Finland", 59.0, 70.5, 20.0, 32.0),
		newBox(prioritySmall, "DK", "Denmark", 54.0, 58.0, 8.0, 15.0),
		newBox(priorityRegional, "PL", "Poland", 49.0, 55.0, 14.0, 25.0),
		newBox(priorityRegional, "CZ", "Czechia", 48.0, 51.2, 12.0, 19.0),
		newBox(priorityRegional, "SK", "Slovakia", 47.5, 50.0, 16.0, 23.0),
		newBox(priorityRegional, "HU", "Hungary", 45.5, 49.5, 16.0, 23.0),
		newBox(priorityRegional, "RO", "Romania", 43.0, 49.5, 20.0, 30.0),
		newBox(priorityRegional, "BG", "Bulgaria", 41.0, 44.5, 22.0, 29.5),
		newBox(priorityRegional, "RS", "Serbia", 42.0, 47.5, 18.0, 23.5),
		newBox(prioritySmall, "BA", "Bosnia and Herzegovina", 42.0, 46.5, 16.0, 19.0),
		newBox(prioritySmall, "ME", "Montenegro", 41.5, 43.8, 18.4, 20.5),
		newBox(prioritySmall, "MK", "North Macedonia", 40.5, 43.0, 20.4, 23.0),
		newBox(prioritySmall, "AL", "Albania", 39.0, 43.0, 19.0, 21.5),
		newBox(priorityRegional, "GR", "Greece", 34.5, 42.5, 19.0, 29.5),
		newBox(prioritySmall, "UA", "Ukraine", 44.0, 53.5, 22.0, 41.5),
		newBox(priorityRegional, "BY", "Belarus", 51.0, 56.5, 23.0, 33.0),
		newBox(prioritySmall, "LT", "Lithuania", 53.5, 56.5, 20.5, 26.0),
		newBox(prioritySmall, "LV", "Latvia", 56.0, 58.5, 20.5, 28.0),
		newBox(prioritySmall, "EE", "Estonia", 57.0, 60.0, 22.0, 28.5),

		// Caucasus and Central Asia.
		newBox(prioritySmall, "GE", "Georgia", 41.0, 44.8, 40.0, 45.5),
		newBox(prioritySmall, "AM", "Armenia", 38.5, 41.5, 43.0, 47.5),
		newBox(priorityRegional, "AZ", "Azerbaijan", 38.5, 42.5, 46.0, 51.0),
		newBox(priorityRegional, "AZ", "Azerbaijan", 38.5, 39.6, 44.5, 45.8),
		newBox(priorityRegional, "TR", "Turkey", 35.0, 43.5, 25.0, 45.0),
		newBox(priorityRegional, "KG", "Kyrgyzstan", 39.0, 44.5, 69.0, 81.5),
		newBox(priorityLarge, "KZ", "Kazakhstan", 40.0, 56.5, 46.0, 88.0),
		newBox(priorityHuge, "RU", "Russia", 54.0, 68.0, 19.0, 31.0),
		newBox(priorityHuge, "RU", "Russia", 52.0, 72.5, 30.0, 180.0),
		newBox(priorityHuge, "RU", "Russia", 43.0, 47.5, 38.0, 48.0),

		// Middle East and North Africa.
		newBox(priorityLarge, "SA", "Saudi Arabia", 15.0, 33.0, 34.0, 56.0),
		newBox(priorityCityState, "AE", "United Arab Emirates", 22.0, 26.5, 51.5, 56.5),
		newBox(priorityEnclave, "QA", "Qatar", 24.0, 26.5, 50.5, 52.0),
		newBox(priorityEnclave, "BH", "Bahrain", 25.5, 26.5, 50.2, 50.8),
		newBox(priorityRegional, "OM", "Oman", 16.0, 26.0, 52.0, 60.0),
		newBox(priorityRegional, "EG", "Egypt", 21.0, 32.5, 24.0, 36.0),
	}
	sort.SliceStable(regions, func(i, j int) bool {
		if regions[i].priority != regions[j].priority {
			return regions[i].priority < regions[j].priority
		}
		return regions[i].area < regions[j].area
	})
	return regions
}

// newBox precomputes the rectangle area so caller code stays tidy.
func newBox(priority int, code, name string, minLat, maxLat, minLon, maxLon float64) regionBox {
	area := (maxLat - minLat) * (maxLon - minLon)
	if area < 0 {
		area = -area
	}
	return regionBox{
		code:     code,
		name:     name,
		minLat:   minLat,
		maxLat:   maxLat,
		minLon:   minLon,
		maxLon:   maxLon,
		priority: priority,
		area:     area,
	}
}
