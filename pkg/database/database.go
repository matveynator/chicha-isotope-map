package database

import (
	"database/sql"
	"fmt"
	"log"
)

// Database представляет интерфейс для взаимодействия с базой данных.
type Database struct {
	DB          *sql.DB    // Подключение к базе данных
	idGenerator chan int64 // Канал для генерации уникальных ID
}

// Config содержит конфигурационные параметры для инициализации базы данных.
type Config struct {
	DBType    string // Тип драйвера базы данных (например, "sqlite", "genji", "pgx" (postgres))
	DBPath    string // Путь к файлу базы данных (для файловых баз данных)
	DBHost    string // Хост для PostgreSQL
	DBPort    int    // Порт для PostgreSQL
	DBUser    string // Пользователь для PostgreSQL
	DBPass    string // Пароль для PostgreSQL
	DBName    string // Имя базы данных PostgreSQL
	PGSSLMode string // Режим SSL для PostgreSQL
	Port      int    // Номер порта (используется при именовании файла базы данных, если необходимо)
}

// startIDGenerator запускает горутину для генерации уникальных ID.
func startIDGenerator(initialID int64) chan int64 {
	idChannel := make(chan int64)
	go func(start int64) {
		currentID := start
		for {
			idChannel <- currentID
			currentID++
		}
	}(initialID)
	return idChannel
}

// NewDatabase создает и инициализирует новое подключение к базе данных.
func NewDatabase(config Config) (*Database, error) {
	var dsn string

	switch config.DBType {
	case "sqlite", "genji":
		dsn = config.DBPath
		if dsn == "" {
			dsn = fmt.Sprintf("database-%d.%s", config.Port, config.DBType)
		}
	case "pgx":
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			config.DBUser, config.DBPass, config.DBHost, config.DBPort, config.DBName, config.PGSSLMode)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", config.DBType)
	}

	db, err := sql.Open(config.DBType, dsn)
	if err != nil {
		return nil, fmt.Errorf("error opening the database: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error connecting to the database: %v", err)
	}

	// Логируем используемый драйвер и DSN
	log.Printf("Using database driver: %s with DSN: %s", config.DBType, dsn)

	// Получаем максимальный текущий ID из таблицы markers
	var maxID sql.NullInt64

	// Запрос для получения максимального ID
	_ = db.QueryRow(`SELECT MAX(id) FROM markers`).Scan(&maxID)

	// Если найдено значение, устанавливаем начальное значение генератора
	initialID := int64(1)
	if maxID.Valid {
		initialID = maxID.Int64 + 1
	}

	// Запускаем генератор ID
	idChannel := startIDGenerator(initialID)

	// Возвращаем экземпляр Database
	return &Database{
		DB:          db,
		idGenerator: idChannel,
	}, nil
}

// InitSchema инициализирует схему базы данных для хранения маркеров.
func (db *Database) InitSchema(config Config) error {
	var schema string

	switch config.DBType {
	case "pgx": // PostgreSQL
		schema = `
		CREATE TABLE IF NOT EXISTS markers (
			id SERIAL PRIMARY KEY,
			doseRate REAL,
			date BIGINT,
			lon REAL,
			lat REAL,
			countRate REAL,
			zoom INTEGER,
			speed REAL,
			trackID VARCHAR(30)
		);
		CREATE INDEX IF NOT EXISTS idx_markers_unique ON markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID);
		CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon);
		CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
		`

	case "sqlite": // SQLite
		schema = `
		CREATE TABLE IF NOT EXISTS markers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			doseRate REAL,
			date INTEGER,
			lon REAL,
			lat REAL,
			countRate REAL,
			zoom INTEGER,
			speed REAL,
			trackID TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_markers_unique ON markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID);
		CREATE INDEX IF NOT EXISTS idx_markers_zoom_bounds ON markers (zoom, lat, lon);
		CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
		`

	case "genji": // Genji
		schema = `
		CREATE TABLE IF NOT EXISTS markers (
			id INTEGER PRIMARY KEY,
			doseRate REAL,
			date INTEGER,
			lon REAL,
			lat REAL,
			countRate REAL,
			zoom INTEGER,
			speed REAL,
			trackID TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_markers_trackid ON markers (trackID);
		`
	default:
		return fmt.Errorf("unsupported database type: %s", config.DBType)
	}

	// Выполняем SQL-запрос для создания таблиц и индексов
	_, err := db.DB.Exec(schema)
	if err != nil {
		return fmt.Errorf("error initializing database schema: %v", err)
	}

	return nil
}

// SaveMarkerAtomic сохраняет маркер атомарно, используя канал генератора ID для уникальных идентификаторов.
func (db *Database) SaveMarkerAtomic(marker Marker, dbType string) error {
	var count int
	var query string

	// Определяем SQL-запрос для проверки существования маркера
	switch dbType {
	case "pgx":
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = $1 AND date = $2 AND lon = $3 AND lat = $4 AND countRate = $5 AND zoom = $6 AND speed = $7 AND trackID = $8`
	default:
		query = `
		SELECT COUNT(1)
		FROM markers
		WHERE doseRate = ? AND date = ? AND lon = ? AND lat = ? AND countRate = ? AND zoom = ? AND speed = ? AND trackID = ?`
	}

	// Выполняем запрос для проверки существования маркера
	err := db.DB.QueryRow(query,
		marker.DoseRate,
		marker.Date,
		marker.Lon,
		marker.Lat,
		marker.CountRate,
		marker.Zoom,
		marker.Speed,
		marker.TrackID).Scan(&count)

	if err != nil {
		return err
	}

	// Если маркер уже существует, не сохраняем его повторно
	if count > 0 {
		log.Printf("Marker (%f, %d, %f, %f, %f, %d, %f, %s) already exists.\n", marker.DoseRate, marker.Date, marker.Lon, marker.Lat, marker.CountRate, marker.Zoom, marker.Speed, marker.TrackID)
		return nil
	}

	// Генерируем уникальный ID, если не используем SERIAL (PostgreSQL)
	if dbType != "pgx" {
		nextID := <-db.idGenerator
		marker.ID = nextID
	}

	// Определяем SQL-запрос для вставки маркера
	switch dbType {
	case "pgx":
		query = `
		INSERT INTO markers (doseRate, date, lon, lat, countRate, zoom, speed, trackID)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	default:
		query = `
		INSERT INTO markers (id, doseRate, date, lon, lat, countRate, zoom, speed, trackID)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	}

	// Выполняем вставку маркера в базу данных
	if dbType == "pgx" {
		_, err = db.DB.Exec(query,
			marker.DoseRate,
			marker.Date,
			marker.Lon,
			marker.Lat,
			marker.CountRate,
			marker.Zoom,
			marker.Speed,
			marker.TrackID)
	} else {
		_, err = db.DB.Exec(query,
			marker.ID,
			marker.DoseRate,
			marker.Date,
			marker.Lon,
			marker.Lat,
			marker.CountRate,
			marker.Zoom,
			marker.Speed,
			marker.TrackID)
	}

	if err != nil {
		return fmt.Errorf("error inserting marker: %v", err)
	}

	return nil
}

// GetMarkersByZoomAndBounds извлекает маркеры, фильтруя их по уровню зума и географическим границам.
func (db *Database) GetMarkersByZoomAndBounds(zoom int, minLat, minLon, maxLat, maxLon float64, dbType string) ([]Marker, error) {
	var query string

	// Определяем SQL-запрос в зависимости от типа базы данных
	switch dbType {
	case "pgx":
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE zoom = $1 AND lat BETWEEN $2 AND $3 AND lon BETWEEN $4 AND $5;`
	default:
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE zoom = ? AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?;`
	}

	// Выполняем запрос с переданными параметрами
	rows, err := db.DB.Query(query, zoom, minLat, maxLat, minLon, maxLon)
	if err != nil {
		return nil, fmt.Errorf("error querying markers: %v", err)
	}
	defer rows.Close()

	var markers []Marker

	// Итерация по результатам запроса и сканирование в структуру Marker
	for rows.Next() {
		var marker Marker
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID)
		if err != nil {
			return nil, fmt.Errorf("error scanning marker: %v", err)
		}
		markers = append(markers, marker)
	}

	// Проверяем наличие ошибок после итерации
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over result set: %v", err)
	}

	return markers, nil
}

// GetMarkersByTrackID возвращает маркеры для конкретного TrackID, отсортированные по дате.
func (db *Database) GetMarkersByTrackID(trackID string, dbType string) ([]Marker, error) {
	var query string

	// Определяем SQL-запрос в зависимости от типа базы данных
	switch dbType {
	case "pgx":
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE trackID = $1
		ORDER BY date ASC;`
	default:
		query = `
		SELECT id, doseRate, date, lon, lat, countRate, zoom, speed, trackID
		FROM markers
		WHERE trackID = ?
		ORDER BY date ASC;`
	}

	// Выполняем запрос с переданным TrackID
	rows, err := db.DB.Query(query, trackID)
	if err != nil {
		return nil, fmt.Errorf("error querying markers by TrackID: %v", err)
	}
	defer rows.Close()

	var markers []Marker

	// Итерация по результатам запроса и сканирование в структуру Marker
	for rows.Next() {
		var marker Marker
		err := rows.Scan(&marker.ID, &marker.DoseRate, &marker.Date, &marker.Lon, &marker.Lat, &marker.CountRate, &marker.Zoom, &marker.Speed, &marker.TrackID)
		if err != nil {
			return nil, fmt.Errorf("error scanning marker: %v", err)
		}
		markers = append(markers, marker)
	}

	// Проверяем наличие ошибок после итерации
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over result set: %v", err)
	}

	return markers, nil
}

