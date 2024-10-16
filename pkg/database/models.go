package database

// Marker представляет данные дозиметра.
type Marker struct {
	ID        int64   `json:"id"` // Добавлено поле ID
	DoseRate  float64 `json:"doseRate"`
	Date      int64   `json:"date"`
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
	CountRate float64 `json:"countRate"`
}

// Data представляет набор данных с маркерами.
type Data struct {
	ID      string   `json:"id"`
	Markers []Marker `json:"markers"`
	Title   string   `json:"title"`
}
