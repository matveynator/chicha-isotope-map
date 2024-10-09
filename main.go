package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

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
	}).ParseFiles("map.html"))

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

	// Маршруты
	http.HandleFunc("/", mapHandler)

	// Запуск сервера
	log.Println("Запуск сервера на :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

