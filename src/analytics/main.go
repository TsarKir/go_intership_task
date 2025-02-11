package main

import (
	"analytics/handler"
	"log"
	"net/http"
)

func main() {
	handler.InitializeRoutes()
	go func() {
		handler.InitKafka()
	}()
	log.Println("Analytics Service is running on port 5555")
	if err := http.ListenAndServe(":5555", nil); err != nil {
		log.Fatal(err)
	}
}
