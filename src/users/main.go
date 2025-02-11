package main

import (
	"log"
	"net/http"
	"users/handler"
)

func main() {
	handler.InitializeRoutes()

	log.Println("User Service is running on port 9999")
	if err := http.ListenAndServe(":9999", nil); err != nil {
		log.Fatal(err)
	}
}
