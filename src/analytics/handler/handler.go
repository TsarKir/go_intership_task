package handler

import (
	"analytics/db"
	"encoding/json"
	"log"
	"os"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type UserKafkaMessage struct {
	Action string `json:"action"`
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

type ProductKafkaMessage struct {
	Action             string `json:"action"`
	UserID             int    `json:"user_id"`
	ProductID          string `json:"product_id"`
	ProductCategory    string `json:"product_category"`
	NumberOfLikes      int    `json:"number_of_likes"`
	ProductDescription string `json:"description"`
	ProductName        string `json:"name"`
}

func InitializeRoutes() {
	db.Connect()
}

func InitKafka() {
	go initUserUpdatesConsumer()
	go initProductUpdatesConsumer()
}

func initUserUpdatesConsumer() {
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": os.Getenv("KAFKA_BROKER"),
		"group.id":          "analytics_user_updates",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		log.Fatalf("Ошибка создания консьюмера для user_updates: %v", err)
	}
	defer consumer.Close()

	// Подписка на топик
	consumer.SubscribeTopics([]string{"user_updates"}, nil)

	// Чтение сообщений
	kafkaLoopUserUpdates(consumer)
}

func initProductUpdatesConsumer() {
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": os.Getenv("KAFKA_BROKER"),
		"group.id":          "analytics_product_updates",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		log.Fatalf("Ошибка создания консьюмера для product_updates: %v", err)
	}
	defer consumer.Close()

	// Подписка на топик
	consumer.SubscribeTopics([]string{"product_updates"}, nil)

	// Чтение сообщений
	kafkaLoopProductUpdates(consumer)
}

func kafkaLoopUserUpdates(consumer *kafka.Consumer) {
	for {
		msg, err := consumer.ReadMessage(-1)
		if err == nil {
			var event UserKafkaMessage
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Error unmarshalling user_updates message: %s", err)
				continue
			}
			processUserKafkaMessage(event)
		} else {
			log.Printf("Error while consuming user_updates message: %s", err)
		}
	}
}

func kafkaLoopProductUpdates(consumer *kafka.Consumer) {
	for {
		msg, err := consumer.ReadMessage(-1)
		if err == nil {
			var event ProductKafkaMessage
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Error unmarshalling product_updates message: %s", err)
				continue
			}
			processProductKafkaMessage(event)
		} else {
			log.Printf("Error while consuming product_updates message: %s", err)
		}
	}
}

func processUserKafkaMessage(event UserKafkaMessage) {
	_, err := db.GetDB().Exec("INSERT INTO user_actions (user_id, action, name, email) VALUES ($1, $2, $3, $4)", event.UserID, event.Action, event.Name, event.Email)
	if err != nil {
		log.Printf("Error while adding user action to database: %s", err)
		return
	}
}

func processProductKafkaMessage(event ProductKafkaMessage) {
	_, err := db.GetDB().Exec("INSERT INTO product_actions (action, user_id, product_id, category, likes, description, name) VALUES ($1, $2, $3, $4, $5, $6, $7)", event.Action, event.UserID, event.ProductID, event.ProductCategory, event.NumberOfLikes, event.ProductDescription, event.ProductName)
	if err != nil {
		log.Printf("Error while adding product action to database: %s", err)
		return
	}
}
