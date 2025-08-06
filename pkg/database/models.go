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

// Data represents a set of markers and associated metadata.
type Data struct {
	ID        string   `json:"id"`      // Unique identifier for the dataset (could be a UUID or string-based ID)
	Markers   []Marker `json:"markers"` // Slice of Marker structs representing individual measurements
	Title     string   `json:"title"`   // Title or description of the dataset
	IsSievert bool     `json:"sv"`      // Indicates if data is in Sievert format
}

// Bounds описывает прямоугольник (minLat,minLon) – (maxLat,maxLon).
type Bounds struct {
	MinLat, MinLon float64
	MaxLat, MaxLon float64
}
