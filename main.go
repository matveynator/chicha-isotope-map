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

// Чтение данных из файла с удалением пустых значений
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

	// Фильтрация маркеров: удаляем маркеры с нулевой или отсутствующей дозой радиации
	var filteredMarkers []Marker
	for _, marker := range data.Markers {
		if marker.DoseRate > 0 {
			filteredMarkers = append(filteredMarkers, marker)
		}
	}

	data.Markers = filteredMarkers
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

