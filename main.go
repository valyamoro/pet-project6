package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/viper"
	"github.com/valyamoro/pkg/database"
)

type Place struct {
	Id int
	Title string
	Slug string
	Address string
	Phone string
	Subway string
	IsClosed bool
	Location string
}

type Serializer[T any] interface {
	Serialize(data []T) ([]byte, error)
	Deserialize(data []byte) (T, error)
}

type JSONSerializer[T any] struct {}

func (js JSONSerializer[T]) Serialize(data []T) ([]byte, error) {
	return json.Marshal(data)
}

func (js JSONSerializer[T]) Deserialize(data []byte) (T, error) {
	var result T 
	err := json.Unmarshal(data, &result)
	return result, err
}

type GobSerializer[T any] struct {}

func (gs GobSerializer[T]) Serialize(data []T) ([]byte, error) {
	var buf bytes.Buffer 
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(data)
	return buf.Bytes(), err 
}

func (gs GobSerializer[T]) Deserialize(data []byte) (T, error) {
	var result T 
	reader := bytes.NewReader(data)
	decoder := gob.NewDecoder(reader)
	err := decoder.Decode(&result)
	return result, err 
}

func GetSerializer[T any](format string) (Serializer[T], error) {
	switch format {
	case "json":
		return JSONSerializer[T]{}, nil
	case "gob":
		return GobSerializer[T]{}, nil
	default:
		return nil, fmt.Errorf("Неизвестный формат сериализации: %s", format)
	}
} 

type App struct {
	DB *pgx.Conn
	LogChan chan ExecutionLog
	Wg *sync.WaitGroup
}

type ExecutionLog struct {
	Id int
	TaskName string 
	StartTime time.Time 
	EndTime time.Time
	DurationSeconds float64
}

func main() {
	envPath := flag.String("env", "", "Путь до файла .env")
	flag.Parse()

	if err := initConfig(*envPath); err != nil {
		log.Fatalf("Ошибка инициализации конфигурации: %v", err)
		return
	}

	ctx := context.Background()
	conn, err := initDB(ctx)
	if err != nil {
		log.Fatal("Не удалось подключиться к базе данных", err)
	}

	defer conn.Close(ctx)

	logChan := make(chan ExecutionLog, 100)
	wg := &sync.WaitGroup{}
	
	app := &App{
		DB: conn,
		LogChan: logChan,
		Wg: wg,
	}

	http.HandleFunc("/all", app.GetAll)

	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Ошибка запуска сервера:", err)
	}

	close(logChan)
	wg.Wait()
	fmt.Println("Сервер завершил работу")
}

func (app *App) storeExecutionLog(el ExecutionLog) error {
	_, err := app.DB.Exec(
		context.Background(),
		"INSERT INTO execution_logs (task_name, start_time, end_time, duration_seconds) VALUES ($1, $2, $3, $4)",
		el.TaskName,
		el.StartTime,
		el.EndTime,
		el.DurationSeconds,
	)

	return err
}

func (app *App) LogExecutionTime(taskName string, action func()) {
	startTime := time.Now()

	action()

	endTime := time.Now()
	duration := endTime.Sub(startTime).Seconds()

	logEntry := ExecutionLog{
		TaskName: taskName,
		StartTime: startTime,
		EndTime: endTime,
		DurationSeconds: duration,
	}

	app.Wg.Add(1)
	app.LogChan <- logEntry
}

func (app *App) GetAll(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	allPlaces, err := fetchAllPlaces()
	if err != nil {
		fmt.Printf("Ошибка: %s", err)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	serializer, err := GetSerializer[Place](format)
	if err != nil {
		fmt.Printf("Ощибка: %s", err)
		return 
	}

	deserializedData, err := serializer.Serialize(allPlaces)
	if err != nil {
		fmt.Printf("Ошибка: %s", err)
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime).Seconds()

	app.storeExecutionLog(ExecutionLog{
		TaskName: "GetAll",
		StartTime: startTime,
		EndTime: endTime,
		DurationSeconds: duration,
	})
	w.Write(deserializedData)
}

func (app *App) proccesLog() {
	for logEntry := range app.LogChan {
		err := app.storeExecutionLog(logEntry)
		if err != nil {
			fmt.Printf("Ощибка записи лога: %s\n", err)
		}

		app.Wg.Done()
	}
}

func fetchAllPlaces() ([]Place, error) {
	const baseURL = "https://kudago.com/public-api/v1.4/places"
	var allPlaces []Place

	client := &http.Client{}

	page := 210
	for {
		url := fmt.Sprintf("%s?page=%d", baseURL, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		} 

		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending request: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading response body: %w", err)
		}

		var result struct {
			Results []Place `json:"results"`
			Next string `json:"next"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("error unmarshalling response: %w", err)
		}

		allPlaces = append(allPlaces, result.Results...)
		
		if result.Next == "" {
			break 
		}

		page++
	}
	
	return allPlaces, nil
}

func initConfig(envPath string) error {
	viper.SetConfigFile(envPath)
	
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("Произошла ошибка: %v", err)
		return err 
	}

	return nil
}

func initDB(ctx context.Context) (*pgx.Conn, error) {
	username := viper.GetString("DB_USERNAME")
	password := viper.GetString("DB_PASSWORD")
	host := viper.GetString("DB_HOST")
	port := viper.GetInt("DB_PORT")
	dbName := viper.GetString("DB_NAME")

	return database.NewPostgresConnection(ctx, database.ConnectionParams{
		Username: username,
		Password: password,
		Host: host,
		Port: port,
		DBName: dbName,
	})
}
