package trackjson

import (
	"net/url"
	"strings"
	"time"

	"chicha-isotope-map/pkg/database"
)

// ==========================
// Shared JSON track helpers
// ==========================

// MicroRoentgenPerMicroSievert translates microsieverts per hour into microroentgen
// per hour. Keeping the constant public allows both the API and archive writers to
// remain in lock-step without sprinkling magic numbers across packages.
const MicroRoentgenPerMicroSievert = 100.0

// Disclaimers lists the language-specific warnings bundled with every export.
// We never mutate the map after init so callers should clone it if they plan to
// tweak messages for custom responses.
var Disclaimers = map[string]string{
	"en": "Data is free to download. We do not create it and take no responsibility for its contents.",
	"ru": "Данные доступны для свободного скачивания. Мы их не создаём и не несём за них никакой ответственности.",
	"es": "Los datos se pueden descargar libremente. No los creamos y no asumimos ninguna responsabilidad por su contenido.",
	"fr": "Les données sont libres de téléchargement. Nous ne les créons pas et n'assumons aucune responsabilité quant à leur contenu.",
	"de": "Die Daten können frei heruntergeladen werden. Wir erstellen sie nicht und übernehmen keine Verantwortung für ihren Inhalt.",
	"pt": "Os dados são livres para download. Não os criamos e não assumimos qualquer responsabilidade pelo seu conteúdo.",
	"it": "I dati sono scaricabili liberamente. Non li creiamo e non ci assumiamo alcuna responsabilità per il loro contenuto.",
	"zh": "数据可自由下载。我们不创建这些数据，对其内容不承担任何责任。",
	"ja": "データは自由にダウンロードできます。私たちはデータを作成しておらず、その内容について一切の責任を負いません。",
	"ar": "البيانات متاحة للتنزيل مجانًا. نحن لا ننشئها ولا نتحمل أي مسؤولية عن محتواها.",
	"hi": "डेटा मुक्त रूप से डाउनलोड किया जा सकता है। हम इसे नहीं बनाते हैं और इसकी सामग्री के लिए कोई ज़िम्मेदारी नहीं लेते हैं।",
	"tr": "Veriler ücretsiz olarak indirilebilir. Biz bu verileri üretmiyoruz ve içeriklerinden hiçbir sorumluluk kabul etmiyoruz.",
	"ko": "데이터는 자유롭게 다운로드할 수 있습니다. 우리는 데이터를 만들지 않으며 그 내용에 대해 어떠한 책임도 지지 않습니다.",
}

// MarkerPayload mirrors the JSON schema served by the API for individual
// measurement points. Reusing the struct keeps archives interchangeable with the
// live endpoints and documents the fields in one place.
type MarkerPayload struct {
	ID                     int64    `json:"id"`
	TimeUnix               int64    `json:"timeUnix"`
	TimeUTC                string   `json:"timeUTC"`
	Lat                    float64  `json:"lat"`
	Lon                    float64  `json:"lon"`
	AltitudeM              *float64 `json:"altitudeM,omitempty"`
	DoseRateMicroSvH       float64  `json:"doseRateMicroSvH"`
	DoseRateMicroRoentgenH float64  `json:"doseRateMicroRh"`
	CountRateCPS           float64  `json:"countRateCPS"`
	SpeedMS                float64  `json:"speedMS"`
	SpeedKMH               float64  `json:"speedKMH"`
	TemperatureC           *float64 `json:"temperatureC,omitempty"`
	HumidityPercent        *float64 `json:"humidityPercent,omitempty"`
	DetectorName           string   `json:"detectorName,omitempty"`
	DetectorType           string   `json:"detectorType,omitempty"`
	RadiationTypes         []string `json:"radiationTypes,omitempty"`
}

// MakeMarkerPayload converts raw database markers into JSON-ready payloads
// while returning the canonical timestamp so callers can reuse it for metadata.
func MakeMarkerPayload(marker database.Marker) (MarkerPayload, time.Time) {
	ts, unixSeconds := normalizeMarkerTime(marker.Date)
	payload := MarkerPayload{
		ID:                     marker.ID,
		TimeUnix:               unixSeconds,
		TimeUTC:                ts.Format(time.RFC3339),
		Lat:                    marker.Lat,
		Lon:                    marker.Lon,
		DoseRateMicroSvH:       marker.DoseRate,
		DoseRateMicroRoentgenH: marker.DoseRate * MicroRoentgenPerMicroSievert,
		CountRateCPS:           marker.CountRate,
		SpeedMS:                marker.Speed,
		SpeedKMH:               marker.Speed * 3.6,
	}
	if marker.AltitudeValid {
		altitude := marker.Altitude
		payload.AltitudeM = &altitude
	}
	if marker.TemperatureValid {
		temperature := marker.Temperature
		payload.TemperatureC = &temperature
	}
	if marker.HumidityValid {
		humidity := marker.Humidity
		payload.HumidityPercent = &humidity
	}
	detector := strings.TrimSpace(marker.Detector)
	if detector != "" {
		payload.DetectorType = detector
		payload.DetectorName = stableDetectorName(marker.TrackID, detector)
	}
	if channels := splitRadiationChannels(marker.Radiation); len(channels) > 0 {
		payload.RadiationTypes = channels
	}
	return payload, ts
}

// TrackAPIPath returns the canonical API URL for fetching the JSON track with a
// .cim extension. Keeping the function here lets the archive embed correct
// references without reaching into HTTP-specific code.
func TrackAPIPath(trackID string) string {
	return "/api/track/" + url.PathEscape(trackID) + ".cim"
}

// SafeCIMFilename normalises track IDs for use as download filenames while
// preserving the .cim suffix expected by downstream tools.
func SafeCIMFilename(trackID string) string {
	var b strings.Builder
	for _, r := range trackID {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "track"
	}
	return name + ".cim"
}

// CopyDisclaimers provides a defensive clone so callers can mutate the map
// without affecting other responses.
func CopyDisclaimers() map[string]string {
	out := make(map[string]string, len(Disclaimers))
	for k, v := range Disclaimers {
		out[k] = v
	}
	return out
}

func normalizeMarkerTime(unixValue int64) (time.Time, int64) {
	if unixValue <= 0 {
		ts := time.Unix(0, 0).UTC()
		return ts, ts.Unix()
	}
	if unixValue > 1_000_000_000_000 {
		ts := time.UnixMilli(unixValue).UTC()
		return ts, ts.Unix()
	}
	ts := time.Unix(unixValue, 0).UTC()
	return ts, unixValue
}

func splitRadiationChannels(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		switch r {
		case ',', ';', '|', '/', '\\':
			return true
		case ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
	seen := make(map[string]struct{})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		channel := strings.ToLower(strings.TrimSpace(f))
		if channel == "" {
			continue
		}
		if _, ok := seen[channel]; ok {
			continue
		}
		seen[channel] = struct{}{}
		out = append(out, channel)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stableDetectorName(trackID, detector string) string {
	trackID = strings.TrimSpace(trackID)
	detector = strings.TrimSpace(detector)
	switch {
	case trackID == "" && detector == "":
		return ""
	case trackID == "":
		return detector
	case detector == "":
		return trackID
	default:
		return trackID + ":" + detector
	}
}
