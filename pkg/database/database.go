package database

import (
  "database/sql"
  "fmt"
  "log"
  _ "github.com/genjidb/genji/driver"
  _ "modernc.org/sqlite"
)

// Database представляет интерфейс для взаимодействия с базой данных.
type Database struct {
  DB *sql.DB
}

// Config содержит конфигурацию для инициализации базы данных.
type Config struct {
  DBType string
  DBPath string
  Port   int
}

// NewDatabase создает и инициализирует новую базу данных.
func NewDatabase(config Config) (*Database, error) {
  var dsn string
  if config.DBType == "sqlite" {
    dsn = config.DBPath
    if dsn == "" {
      dsn = fmt.Sprintf("database-%d.sqlite", config.Port)

    }
  } else {
    dsn = config.DBPath
    if dsn == "" {
      dsn = fmt.Sprintf("database-%d.genji", config.Port)
    }
  }

  db, err := sql.Open(config.DBType, dsn)
  if err != nil {
    return nil, fmt.Errorf("ошибка открытия базы данных: %v", err)
  }

  if err := db.Ping(); err != nil {
    return nil, fmt.Errorf("ошибка подключения к базе данных: %v", err)
  }

  log.Printf("Используется база данных: %s по адресу %s", config.DBType, dsn)

  return &Database{DB: db}, nil
}

// InitSchema инициализирует схему таблиц для хранения данных.
func (db *Database) InitSchema() error {
  schema := `
  CREATE TABLE IF NOT EXISTS markers (
    id INTEGER PRIMARY KEY,
    doseRate REAL,
    date INTEGER,
    lon REAL,
    lat REAL,
    countRate REAL
  );
  `
  _, err := db.DB.Exec(schema)
  return err
}

// getNextID находит максимальный текущий id и возвращает следующий.
func (db *Database) getNextID() (int64, error) {
  var maxID sql.NullInt64
  err := db.DB.QueryRow(`SELECT MAX(id) FROM markers`).Scan(&maxID)
  if err != nil {
    return 0, fmt.Errorf("ошибка получения максимального ID: %v", err)
  }

  if maxID.Valid {
    return maxID.Int64 + 1, nil
  }
  return 1, nil // Если нет записей, начинаем с 1.
}

// SaveMarker сохраняет маркер в базу данных с новым ID.
func (db *Database) SaveMarker(marker Marker) error {
  nextID, err := db.getNextID()
  if err != nil {
    return fmt.Errorf("ошибка получения следующего ID: %v", err)
  }

  _, err = db.DB.Exec(`
  INSERT INTO markers (id, doseRate, date, lon, lat, countRate)
  VALUES (?, ?, ?, ?, ?, ?)
  `, nextID, marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate)

  return err
}

// LoadMarkers загружает все маркеры из базы данных.
func (db *Database) LoadMarkers() ([]Marker, error) {
  rows, err := db.DB.Query(`
  SELECT id, doseRate, date, lon, lat, countRate FROM markers
  `)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  var markers []Marker
  for rows.Next() {
    var marker Marker
    if err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate); err != nil {
      return nil, err
    }
    markers = append(markers, marker)
  }
  return markers, nil
}

