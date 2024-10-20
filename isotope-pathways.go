package main

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"isotope-pathways/pkg/config"
	"isotope-pathways/pkg/database"
)

// Встраивание файлов из папки public_html
//
//go:embed public_html/*
var content embed.FS

// Переменная для хранения загруженных данных
var doseData database.Data

var dbType = flag.String("db-type", "genji", "Тип базы данных: genji или sqlite")
var dbPath = flag.String("db-path", "", "Путь к файлу базы данных (по умолчанию в текущей папке)")
var port = flag.Int("port", 8765, "Порт для запуска сервера")
var version = flag.Bool("version", false, "Показать версию программы")

var db *database.Database

// Функция для проверки, совпадают ли два маркера по всем полям
func areMarkersEqual(m1, m2 database.Marker) bool {
	return m1.DoseRate == m2.DoseRate &&
		m1.Date == m2.Date &&
		m1.Lon == m2.Lon &&
		m1.Lat == m2.Lat &&
		m1.CountRate == m2.CountRate
}

// Фильтрация маркеров: удаляем маркеры с нулевой дозой радиации и дубликаты
func filterUniqueMarkers(markers []database.Marker) []database.Marker {
	var filteredMarkers []database.Marker

	for _, newMarker := range markers {
		if newMarker.DoseRate == 0 {
			continue // Игнорируем маркеры с нулевой дозой
		}

		isDuplicate := false
		for _, existingMarker := range filteredMarkers {
			if areMarkersEqual(newMarker, existingMarker) {
				isDuplicate = true
				break
			}
		}

		if !isDuplicate {
			filteredMarkers = append(filteredMarkers, newMarker)
		}
	}

	return filteredMarkers
}

// Чтение данных из файла с удалением пустых значений и дубликатов
func loadDataFromFile(filename string) (database.Data, error) {
	var data database.Data

	// Чтение содержимого файла
	file, err := os.Open(filename)
	if err != nil {
		return data, err
	}
	defer file.Close()

	// Чтение файла в байты
	byteValue, err := ioutil.ReadAll(file)
	if err != nil {
		return data, err
	}

	// Парсинг JSON данных
	err = json.Unmarshal(byteValue, &data)
	if err != nil {
		return data, err
	}

	// Фильтрация уникальных маркеров
	data.Markers = filterUniqueMarkers(data.Markers)

	return data, nil
}

// Функция для парсинга строки в float64
func parseFloat(value string) float64 {
	parsedValue, _ := strconv.ParseFloat(value, 64)
	return parsedValue
}

var translations map[string]map[string]string

func loadTranslations(filename string) {
	file, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("Ошибка чтения файла переводов: %v", err)
	}

	err = json.Unmarshal(file, &translations)
	if err != nil {
		log.Fatalf("Ошибка парсинга переводов: %v", err)
	}
}

func getPreferredLanguage(r *http.Request) string {
	// Заголовок Accept-Language может содержать несколько языков с приоритетами, например: "en-US,en;q=0.9,fr;q=0.8"
	langHeader := r.Header.Get("Accept-Language")

	// Поддерживаемые языки
	supportedLanguages := []string{"en", "zh", "es", "hi", "ar", "fr", "ru", "pt", "de", "ja", "tr", "it", "ko", "pl", "uk", "mn", "no", "fi", "ka", "sv", "he", "nl", "el", "hu", "cs", "ro", "th", "vi", "id", "ms", "bg", "lt", "et", "lv", "sl"}

	// Разбиваем заголовок на языки с приоритетами
	langs := strings.Split(langHeader, ",")

	for _, lang := range langs {
		// Извлекаем сам язык без приоритета (после "q=")
		lang = strings.TrimSpace(strings.SplitN(lang, ";", 2)[0])

		// Пробуем найти полное совпадение с поддерживаемым языком
		for _, supported := range supportedLanguages {
			if strings.HasPrefix(lang, supported) {
				return supported
			}
		}
	}

	// Если ни один язык не поддерживается, возвращаем язык по умолчанию — английский
	return "en"
}

// Вспомогательные функции для извлечения дозы радиации и счетчика из описания
func extractDoseRate(description string) float64 {
	re := regexp.MustCompile(`(\d+(\.\d+)?) µR/h`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		return parseFloat(match[1]) / 100 // Преобразуем из µR/h в µSv/h
	}
	return 0
}

func extractCountRate(description string) float64 {
	re := regexp.MustCompile(`(\d+(\.\d+)?) cps`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		return parseFloat(match[1])
	}
	return 0
}

// Функция для приблизительного определения временной зоны по долготе
func getTimeZoneByLongitude(lon float64) *time.Location {
	switch {
	case lon >= 37 && lon <= 60: // Москва и часть России
		loc, _ := time.LoadLocation("Europe/Moscow")
		return loc
	case lon >= -9 && lon <= 3: // Центральная Европа
		loc, _ := time.LoadLocation("Europe/Berlin")
		return loc
	case lon >= -180 && lon < -60: // Северная Америка
		loc, _ := time.LoadLocation("America/New_York")
		return loc
	default: // По умолчанию UTC
		loc, _ := time.LoadLocation("UTC")
		return loc
	}
}

// Функция для парсинга времени в формате "Feb 3, 2024 19:44:03"
func parseDate(description string, loc *time.Location) int64 {
	re := regexp.MustCompile(`<b>([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})<\/b>`)
	match := re.FindStringSubmatch(description)
	if len(match) > 0 {
		dateString := match[1]
		layout := "Jan 2, 2006 15:04:05"
		t, err := time.ParseInLocation(layout, dateString, loc)
		if err == nil {
			return t.Unix() // Возвращаем время в формате UNIX timestamp
		}
		log.Println("Ошибка парсинга даты:", err)
	}
	return 0
}

// Функция для парсинга KML файла
func parseKML(data []byte) ([]database.Marker, error) {
	var markers []database.Marker
	var longitudes []float64

	coordinatePattern := regexp.MustCompile(`<coordinates>(.*?)<\/coordinates>`)
	descriptionPattern := regexp.MustCompile(`<description><!\[CDATA\[(.*?)\]\]><\/description>`)

	coordinates := coordinatePattern.FindAllStringSubmatch(string(data), -1)
	descriptions := descriptionPattern.FindAllStringSubmatch(string(data), -1)

	// Сбор всех долгот для определения временной зоны
	for i := 0; i < len(coordinates) && i < len(descriptions); i++ {
		coords := strings.Split(strings.TrimSpace(coordinates[i][1]), ",")
		if len(coords) >= 2 {
			lon := parseFloat(coords[0])
			lat := parseFloat(coords[1])
			longitudes = append(longitudes, lon)

			doseRate := extractDoseRate(descriptions[i][1])
			countRate := extractCountRate(descriptions[i][1])
			// Мы ещё не знаем точную временную зону, так что время пока не парсим
			marker := database.Marker{
				DoseRate:  doseRate,
				Lat:       lat,
				Lon:       lon,
				CountRate: countRate,
			}
			markers = append(markers, marker)
		}
	}

	// Определение средней долготы для временной зоны
	var avgLon float64
	for _, lon := range longitudes {
		avgLon += lon
	}
	avgLon /= float64(len(longitudes))

	// Получаем временную зону по среднему значению долготы
	loc := getTimeZoneByLongitude(avgLon)

	// Теперь пересчитываем время для каждого маркера
	for i := range markers {
		markers[i].Date = parseDate(descriptions[i][1], loc)
	}

	return markers, nil
}

// Обработка KML файла
func processKMLFile(file multipart.File) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Ошибка чтения KML файла:", err)
		return
	}

	markers, err := parseKML(data)
	if err != nil {
		log.Println("Ошибка парсинга KML файла:", err)
		return
	}

	uniqueMarkers := filterUniqueMarkers(markers)
	doseData.Markers = append(doseData.Markers, uniqueMarkers...)

	// Сохраняем маркеры в базу данных
	for _, marker := range uniqueMarkers {
		if err := db.SaveMarker(marker); err != nil {
			log.Printf("Ошибка сохранения маркера в БД: %v", err)
		}
	}
}

// Обработка KMZ файла
func processKMZFile(file multipart.File) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Ошибка чтения KMZ файла:", err)
		return
	}

	// Открытие KMZ как zip-архива
	zipReader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		log.Println("Ошибка при открытии KMZ файла как ZIP:", err)
		return
	}

	// Ищем KML файл внутри архива
	for _, zipFile := range zipReader.File {
		if filepath.Ext(zipFile.Name) == ".kml" {
			kmlFile, err := zipFile.Open()
			if err != nil {
				log.Println("Ошибка при открытии KML файла внутри KMZ:", err)
				continue
			}
			defer kmlFile.Close()

			// Чтение содержимого KML файла
			kmlData, err := io.ReadAll(kmlFile)
			if err != nil {
				log.Println("Ошибка при чтении KML файла внутри KMZ:", err)
				return
			}

			// Парсим KML данные
			markers, err := parseKML(kmlData)
			if err != nil {
				log.Println("Ошибка парсинга KML файла из KMZ:", err)
				return
			}

			// Добавляем к существующим данным
			doseData.Markers = append(doseData.Markers, filterUniqueMarkers(markers)...)

			// Сохраняем маркеры в базу данных
			for _, marker := range markers {
				if err := db.SaveMarker(marker); err != nil {
					log.Printf("Ошибка сохранения маркера в БД: %v", err)
				}
			}
		}
	}
}

// Обработка текстового формата RCTRK с проверками на корректность данных
func parseTextRCTRK(data []byte) ([]database.Marker, error) {
	var markers []database.Marker
	lines := strings.Split(string(data), "\n")

	// Пропускаем строки, которые не содержат данные
	for i, line := range lines {
		// Пропускаем заголовок и пустые строки
		if i == 0 || strings.HasPrefix(line, "Timestamp") || strings.TrimSpace(line) == "" {
			continue
		}

		// Разбиваем строку на поля
		fields := strings.Fields(line)
		if len(fields) < 7 {
			log.Printf("Пропуск строки %d: недостаточно полей. Строка: %s\n", i+1, line)
			continue // Если в строке недостаточно данных, пропускаем её
		}

		// Парсим данные
		timeStampStr := fields[1] + " " + fields[2] // Поле Time разделено на дату и время
		layout := "2006-01-02 15:04:05"
		parsedTime, err := time.Parse(layout, timeStampStr)
		if err != nil {
			log.Printf("Пропуск строки %d: ошибка парсинга времени. Строка: %s, Ошибка: %v\n", i+1, line, err)
			continue
		}

		// Парсим широту и долготу
		lat := parseFloat(fields[3])
		lon := parseFloat(fields[4])
		if lat == 0 || lon == 0 {
			log.Printf("Пропуск строки %d: некорректные координаты. Latitude: %v, Longitude: %v\n", i+1, lat, lon)
			continue
		}

		// Парсим DoseRate и CountRate
		doseRate := parseFloat(fields[6])
		countRate := parseFloat(fields[7])
		if doseRate < 0 || countRate < 0 {
			log.Printf("Пропуск строки %d: некорректные значения DoseRate или CountRate. DoseRate: %v, CountRate: %v\n", i+1, doseRate, countRate)
			continue
		}

		// Создаем маркер
		marker := database.Marker{
			DoseRate:  doseRate / 100, // Преобразуем в корректную единицу
			CountRate: countRate,
			Lat:       lat,
			Lon:       lon,
			Date:      parsedTime.Unix(), // Время в формате UNIX timestamp
		}
		markers = append(markers, marker)
	}

	return markers, nil
}

// Модификация функции processRCTRKFile для поддержки как JSON, так и текстового формата
func processRCTRKFile(file multipart.File) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Ошибка чтения RCTRK файла:", err)
		return
	}

	// Пробуем сначала парсить как JSON
	var rctrkData database.Data
	err = json.Unmarshal(data, &rctrkData)
	if err == nil {
		// Если парсинг JSON успешен, продолжаем
		rctrkData.Markers = filterUniqueMarkers(rctrkData.Markers)
		doseData.Markers = append(doseData.Markers, rctrkData.Markers...)
	} else {
		// Если это не JSON, пробуем парсить как текстовый файл
		markers, err := parseTextRCTRK(data)
		if err != nil {
			log.Println("Ошибка парсинга текстового RCTRK файла:", err)
			return
		}
		uniqueMarkers := filterUniqueMarkers(markers)
		doseData.Markers = append(doseData.Markers, uniqueMarkers...)
	}

	// Сохраняем маркеры в базу данных
	for _, marker := range doseData.Markers {
		if err := db.SaveMarker(marker); err != nil {
			log.Printf("Ошибка сохранения маркера в БД: %v", err)
		}
	}
}

// Функция для отображения карты
func mapHandler(w http.ResponseWriter, r *http.Request) {
	lang := getPreferredLanguage(r)

	// Добавляем функцию toJSON для использования в шаблоне
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"translate": func(key string) string {
			if val, ok := translations[lang][key]; ok {
				return val
			}
			return translations["en"][key] // если не найдено, показываем на английском
		},
		"toJSON": func(data interface{}) (string, error) {
			bytes, err := json.Marshal(data)
			return string(bytes), err
		},
	}).ParseFS(content, "public_html/map.html"))

	if config.CompileVersion == "dev" {
		config.CompileVersion = "latest"
	}

	data := struct {
		Markers      []database.Marker
		Version      string
		Translations map[string]string
	}{
		Markers:      doseData.Markers,
		Version:      config.CompileVersion,
		Translations: translations[lang],
	}

	err := tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Обработка файла AtomFast JSON
func processAtomFastFile(file multipart.File) {
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Ошибка чтения JSON AtomFast файла:", err)
		return
	}

	var atomFastData []struct {
		DV  int     `json:"dv"`  // Устройство
		D   float64 `json:"d"`   // Уровень радиации (µSv/h)
		R   int     `json:"r"`   // GPS accuracy
		Lat float64 `json:"lat"` // Широта
		Lng float64 `json:"lng"` // Долгота
		T   int64   `json:"t"`   // Время в формате Unix (мс)
	}

	err = json.Unmarshal(data, &atomFastData)
	if err != nil {
		log.Println("Ошибка парсинга AtomFast файла:", err)
		return
	}

	var markers []database.Marker
	for _, record := range atomFastData {
		marker := database.Marker{
			DoseRate:  record.D,
			Date:      record.T / 1000, // Преобразуем миллисекунды в секунды
			Lon:       record.Lng,
			Lat:       record.Lat,
			CountRate: record.D * 100, // Устройство AtomFast не предоставляет CPS но по сути доза у них является CPS.
		}
		markers = append(markers, marker)
	}

	// Добавляем уникальные маркеры и сохраняем их в базу данных
	uniqueMarkers := filterUniqueMarkers(markers)
	doseData.Markers = append(doseData.Markers, uniqueMarkers...)

	for _, marker := range uniqueMarkers {
		if err := db.SaveMarker(marker); err != nil {
			log.Printf("Ошибка сохранения маркера в БД: %v", err)
		}
	}
}

// Обработчик загрузки нескольких файлов
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(100 << 20) // Ограничиваем до 100MB
	if err != nil {
		http.Error(w, "Ошибка загрузки файла", http.StatusInternalServerError)
		return
	}

	// Получаем список файлов
	files := r.MultipartForm.File["files[]"] // Это имя параметра, который мы передаем с клиента

	if len(files) == 0 {
		http.Error(w, "Файлы не выбраны", http.StatusBadRequest)
		return
	}

	for _, fileHeader := range files {
		// Открываем файл
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, "Ошибка открытия файла", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Определяем тип файла по расширению
		ext := filepath.Ext(fileHeader.Filename)
		switch ext {
		case ".kml":
			processKMLFile(file)
		case ".kmz":
			processKMZFile(file)
		case ".rctrk":
			processRCTRKFile(file)
		case ".json":
			processAtomFastFile(file) // Обрабатываем AtomFast JSON
		default:
			http.Error(w, "Неподдерживаемый тип файла", http.StatusBadRequest)
			return
		}
	}

	// Успешная загрузка
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// Основная функция запуска сервера
func main() {
	flag.Parse()

	// Загрузка переводов из файла
	loadTranslations("translations.json")

	// Обработка флага версии
	if *version {
		fmt.Printf("isotope-pathways версия %s\n", config.CompileVersion)
		return
	}

	// Инициализация базы данных
	dbConfig := database.Config{
		DBType: *dbType,
		DBPath: *dbPath,
		Port:   *port,
	}

	var err error
	db, err = database.NewDatabase(dbConfig)
	if err != nil {
		log.Fatalf("Не удалось инициализировать базу данных: %v", err)
	}

	// Инициализация схемы таблиц
	if err := db.InitSchema(); err != nil {
		log.Fatalf("Ошибка при инициализации схемы БД: %v", err)
	}

	// Загрузка данных из базы данных
	markers, err := db.LoadMarkers()
	if err != nil {
		log.Printf("Ошибка загрузки маркеров из БД: %v", err)
	} else {
		doseData.Markers = append(doseData.Markers, filterUniqueMarkers(markers)...)
	}

	// Запуск веб-сервера

	// Настраиваем файловый сервер для раздачи содержимого public_html по пути /static
	staticFiles, err := fs.Sub(content, "public_html")
	if err != nil {
		log.Fatalf("Не удалось выделить поддиректорию public_html: %v", err)
	}

	// Раздаём файлы через /static
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFiles))))

	http.HandleFunc("/", mapHandler)
	http.HandleFunc("/upload", uploadHandler)
	log.Printf("Приложение работает по адресу: http://localhost:%d", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
