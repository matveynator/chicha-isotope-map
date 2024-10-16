package main

import (
	"time"
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

//	"github.com/webview/webview_go"
)

// Встраивание файлов из папки public_html
//go:embed public_html/*
var content embed.FS

// Структура для хранения данных дозиметра
type Marker struct {
	DoseRate  float64 `json:"doseRate"`
	Date      int64   `json:"date"`
	Lon       float64 `json:"lon"`
	Lat       float64 `json:"lat"`
	CountRate float64 `json:"countRate"`
}

type Data struct {
	ID      string   `json:"id"`
	Markers []Marker `json:"markers"`
	Title   string   `json:"title"`
}

// Переменная для хранения загруженных данных
var doseData Data

// Функция для проверки, совпадают ли два маркера по всем полям
func areMarkersEqual(m1, m2 Marker) bool {
	return m1.DoseRate == m2.DoseRate &&
		m1.Date == m2.Date &&
		m1.Lon == m2.Lon &&
		m1.Lat == m2.Lat &&
		m1.CountRate == m2.CountRate
}

// Фильтрация маркеров: удаляем маркеры с нулевой дозой радиации и дубликаты
func filterUniqueMarkers(markers []Marker) []Marker {
	var filteredMarkers []Marker

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
func loadDataFromFile(filename string) (Data, error) {
	var data Data

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

// Функция для приблизительного определения временной зоны по долготе
func getTimeZoneByLongitude(lon float64) *time.Location {
    // Примерное распределение часовых поясов по долготе
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
// Обновим функцию parseDate, чтобы учитывать временную зону на основе долготы
func parseDate(description string, loc *time.Location) int64 {
    re := regexp.MustCompile(`<b>([A-Za-z]{3} \d{1,2}, \d{4} \d{2}:\d{2}:\d{2})<\/b>`)
    match := re.FindStringSubmatch(description)
    if len(match) > 0 {
        dateString := match[1]
        layout := "Jan 2, 2006 15:04:05"
        t, err := time.ParseInLocation(layout, dateString, loc)
        if err == nil {
            return t.Unix() // Возвращаем время в формате UNIX timestamp
        } else {
            log.Println("Ошибка парсинга даты:", err)
        }
    }
    return 0
}

// Обновляем функцию parseKML для определения временной зоны на основе координат
func parseKML(data []byte) ([]Marker, error) {
    var markers []Marker
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
            marker := Marker{
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

func parseFloat(value string) float64 {
	parsedValue, _ := strconv.ParseFloat(value, 64)
	return parsedValue
}

// Функция для преобразования данных в JSON
func toJSON(data interface{}) (string, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Обработка KML файла
func processKMLFile(file multipart.File) {
    // Чтение данных файла
    data, err := ioutil.ReadAll(file)
    if err != nil {
        log.Println("Ошибка чтения KML файла:", err)
        return
    }

    // Парсим KML данные
    markers, err := parseKML(data)
    if err != nil {
        log.Println("Ошибка парсинга KML файла:", err)
        return
    }

    // Добавляем к существующим данным
    doseData.Markers = append(doseData.Markers, filterUniqueMarkers(markers)...)
}


// Функция для отдачи HTML-страницы
func mapHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.New("map.html").Funcs(template.FuncMap{
		"toJSON": toJSON,
	}).ParseFS(content, "public_html/map.html"))

	// Выполняем шаблон
	err := tmpl.Execute(w, doseData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Обработчик загрузки файла
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // Ограничиваем до 10MB
	if err != nil {
		http.Error(w, "Ошибка загрузки файла", http.StatusInternalServerError)
		return
	}

	// Получаем файл
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Не удалось загрузить файл", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Определяем тип файла по расширению
	ext := filepath.Ext(header.Filename)
	switch ext {
	case ".kml":
		processKMLFile(file)
	case ".kmz":
		processKMZFile(file)
	case ".rctrk":
		processRCTRKFile(file)
	default:
		http.Error(w, "Неподдерживаемый тип файла", http.StatusBadRequest)
		return
	}

	// Успешная загрузка
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// Обработка KMZ файла
func processKMZFile(file multipart.File) {
	// Чтение данных файла
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
		}
	}
}

// Обработка RCTRK файла
func processRCTRKFile(file multipart.File) {
	// Чтение данных файла
	data, err := ioutil.ReadAll(file)
	if err != nil {
		log.Println("Ошибка чтения RCTRK файла:", err)
		return
	}

	// Парсинг RCTRK данных в структуру Data
	var rctrkData Data
	err = json.Unmarshal(data, &rctrkData)
	if err != nil {
		log.Println("Ошибка парсинга RCTRK файла:", err)
		return
	}

	// Фильтрация уникальных маркеров
	rctrkData.Markers = filterUniqueMarkers(rctrkData.Markers)

    // Добавляем новые маркеры к существующим данным
    doseData.Markers = append(doseData.Markers, rctrkData.Markers...)
}

func main() {
    // Загрузка данных из файла
    var err error
    doseData, err = loadDataFromFile("isotope-map-kmv-northcaucases.rctrk")
    if err != nil {
        log.Fatalf("Ошибка при чтении файла: %v", err)
    }

    fmt.Println("Данные успешно загружены:", doseData.Title)

    // Запуск веб-сервера
        http.HandleFunc("/", mapHandler)
        http.HandleFunc("/upload", uploadHandler)
        log.Println("Запуск сервера на :8765...")
        log.Fatal(http.ListenAndServe(":8765", nil))

}

