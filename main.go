package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/webview/webview_go"
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


// Функция для преобразования данных в JSON
func toJSON(data interface{}) (string, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
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

func main() {
	// Загрузка данных из файла
	var err error
	doseData, err = loadDataFromFile("isotope-map-kmv-northcaucases.rctrk")
	if err != nil {
		log.Fatalf("Ошибка при чтении файла: %v", err)
	}

	fmt.Println("Данные успешно загружены:", doseData.Title)

	// Запуск веб-сервера
	go func() {
		http.HandleFunc("/", mapHandler)
		log.Println("Запуск сервера на :8080...")
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	// Запуск встроенного браузера с использованием библиотеки webview
	debug := true
	w := webview.New(debug)
	defer w.Destroy()
	w.SetTitle("Isotope Pathways")
	w.SetSize(800, 700, webview.HintNone)
	w.Navigate("http://localhost:8080")
	w.Run()
}

