package database

// Marker represents dosimeter data for a specific location or point.
type Marker struct {
	ID        int64   `json:"id"`        // Unique identifier for the marker (added for database purposes)
	DoseRate  float64 `json:"doseRate"`  // The radiation dose rate in µSv/h (microsieverts per hour)
	Date      int64   `json:"date"`      // Timestamp of the measurement (in UNIX time format)
	Lon       float64 `json:"lon"`       // Longitude of the location where the measurement was taken
	Lat       float64 `json:"lat"`       // Latitude of the location where the measurement was taken
	CountRate float64 `json:"countRate"` // Count rate of the measurement (CPS - counts per second)
	Zoom      int     `json:"zoom"`      // Zoom level
	Speed     float64 `json:"speed"`     // Speed of the measurement point
	TrackID   string  `json:"trackID"`   // Identifier of the track
}

type Data struct {
	ID      string   `json:"id"`
	Markers []Marker `json:"markers"`
	Title   string   `json:"title"`
	// NEW — принимаем оба варианта имён
	IsSievert       bool `json:"sv"`        // новое поле Radiacode-Android
	IsSievertLegacy bool `json:"isSievert"` // старые iOS-дампы
}

// Bounds описывает прямоугольник (minLat,minLon) – (maxLat,maxLon).
type Bounds struct {
	MinLat, MinLon float64
	MaxLat, MaxLon float64
}

// RealtimeMeasurement keeps the latest network readings.
// We store raw numbers so the history can be rendered later.
type RealtimeMeasurement struct {
	ID         int64   `json:"id"`         // Primary key for database storage
	DeviceID   string  `json:"deviceID"`   // Remote device identifier
	Value      float64 `json:"value"`      // Reported radiation value
	Unit       string  `json:"unit"`       // Measurement unit from the device
	Lat        float64 `json:"lat"`        // Device latitude
	Lon        float64 `json:"lon"`        // Device longitude
	MeasuredAt int64   `json:"measuredAt"` // Timestamp supplied by the device
	FetchedAt  int64   `json:"fetchedAt"`  // When we pulled it, aids freshness checks
}
