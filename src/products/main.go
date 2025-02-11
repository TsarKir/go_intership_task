package main

import (
	"log"
	"net/http"
	handler "products/handler"
)

func main() {
	handler.InitializeRoutes()

	log.Println("Product Service is running on port 7777")
	if err := http.ListenAndServe(":7777", nil); err != nil {
		log.Fatal(err)
	}
}
