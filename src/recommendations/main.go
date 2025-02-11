package main

import (
	"log"
	"net/http"
	"recommendations/handler"
)

func main() {
	handler.InitializeRoutes()
	go func() {
		handler.InitKafka()
	}()
	log.Println("Recommendation Service is running on port 6666")
	if err := http.ListenAndServe(":6666", nil); err != nil {
		log.Fatal(err)
	}
}
