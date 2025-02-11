package db

import (
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq" // Импортируем драйвер pq
)

var db *sql.DB

// Connect устанавливает подключение к базе данных
func Connect() {
	var err error
	connStr := os.Getenv("DATABASE_URL") // Замените на ваши данные
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}

	// Проверка подключения
	if err = db.Ping(); err != nil {
		log.Fatalf("Cannot ping database: %v\n", err)
	}
}

// GetDB возвращает текущее подключение к базе данных
func GetDB() *sql.DB {
	return db
}
