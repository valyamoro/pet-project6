package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/spf13/viper"
	"github.com/valyamoro/pkg/database"
)

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
