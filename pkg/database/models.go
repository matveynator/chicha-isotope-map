package database

// Marker представляет данные дозиметра для конкретного местоположения или точки.
type Marker struct {
  ID        int64   `json:"id"`        // Уникальный идентификатор для маркера
  DoseRate  float64 `json:"doseRate"`  // Уровень радиации в µSv/h
  Date      int64   `json:"date"`      // Временная метка измерения (UNIX time)
  Lon       float64 `json:"lon"`       // Долгота местоположения
  Lat       float64 `json:"lat"`       // Широта местоположения
  CountRate float64 `json:"countRate"` // Частота счёта измерения (CPS)
  Zoom      int     `json:"zoom"`      // Уровень зума
  Speed     float64 `json:"speed"`     // Скорость
  TrackID   string  `json:"trackID"`   // Идентификатор трека
}

// Data представляет набор маркеров и связанную с ними метаинформацию.
type Data struct {
  ID        string   `json:"id"`      // Уникальный идентификатор набора данных
  Markers   []Marker `json:"markers"` // Срез маркеров
  Title     string   `json:"title"`   // Заголовок или описание набора данных
  IsSievert bool     `json:"sv"`      // Является ли данные в Сивертах
}

