package database

// Marker represents dosimeter data for a specific location or point.
type Marker struct {
	ID						int64   `json:"id"`        // Unique identifier for the marker (added for database purposes)
	DoseRate			float64 `json:"doseRate"`  // The radiation dose rate in µSv/h (microsieverts per hour)
	Date					int64   `json:"date"`      // Timestamp of the measurement (in UNIX time format)
	Lon						float64 `json:"lon"`       // Longitude of the location where the measurement was taken
	Lat						float64 `json:"lat"`       // Latitude of the location where the measurement was taken
	CountRate			float64 `json:"countRate"` // Count rate of the measurement (CPS - counts per second)
	Zoom      		int     `json:"zoom"`
	Speed     		float64 `json:"speed"` 

}

// Data represents a set of markers and associated metadata.
type Data struct {
	ID          string   `json:"id"`      // Unique identifier for the dataset (could be a UUID or string-based ID)
	Markers     []Marker `json:"markers"` // Slice of Marker structs representing individual measurements
	Title       string   `json:"title"`   // Title or description of the dataset
	IsSievert   bool     `json:"sv"`      // Is data in Sievert (1) or Roentgen (1/100)  format?     
}
