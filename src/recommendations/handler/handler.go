package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"recommendations/db"
	"strconv"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-redis/redis/v8"
)

var ctx = context.Background()

// Инициализация клиента Redis
var redisClient = redis.NewClient(&redis.Options{
	Addr: os.Getenv("REDIS_URL"), // Адрес вашего Redis-сервера
})

// Порог для кеширования
const cacheThreshold = 5

type KafkaMessage struct {
	Action             string `json:"action"`
	UserID             int    `json:"user_id"`
	ProductID          string `json:"product_id"`
	ProductCategory    string `json:"product_category"`
	NumberOfLikes      int    `json:"number_of_likes"`
	ProductDescription string `json:"description"`
	ProductName        string `json:"name"`
}

type Product struct {
	ID       int
	Category string
	Likes    int
}

type RecommendationRequest struct {
	UserID    int `json:"user_id"`
	ProductID int `json:"product_id"`
}

type RecommendationResponce struct {
	ProductID int `json:"id"`
}

func recommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	req, err := parseRequest(r)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	response, err := getRecommendationsFromCache(req.UserID, req.ProductID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting recommendations from cache: %v", err), http.StatusInternalServerError)
		return
	}
	if response != nil {
		sendResponse(w, response)
		return
	}

	incrementRequestCount(req.UserID, req.ProductID)
	requestCount := getRequestCount(req.UserID, req.ProductID)

	if isRecommendationInDB(req.UserID, req.ProductID) {
		response = getRecommendationFromDB(req.UserID, req.ProductID)
	} else {
		recommendations, err := getRecommendations(req.UserID, req.ProductID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error getting recommendations: %v", err), http.StatusInternalServerError)
			return
		}

		response = convertRecommendations(recommendations)

		addRecommendationToBD(req.UserID, req.ProductID, response)
	}
	if requestCount > cacheThreshold {
		cacheRecommendations(req.UserID, req.ProductID, response)
	}

	sendResponse(w, response)
}

func convertRecommendations(recommendations []Product) []RecommendationResponce {
	response := make([]RecommendationResponce, len(recommendations))
	for i, product := range recommendations {
		response[i].ProductID = product.ID
	}
	return response
}

func cacheRecommendations(userID int, productID int, recommendations []RecommendationResponce) {
	cacheKey := fmt.Sprintf("recommendations:%d:%d", userID, productID)
	recommendationsJSON, err := json.Marshal(recommendations)
	if err == nil {
		redisClient.Set(ctx, cacheKey, recommendationsJSON, 10*time.Minute)
	}
}

func getRecommendationsFromCache(userID int, productID int) ([]RecommendationResponce, error) {
	cacheKey := fmt.Sprintf("recommendations:%d:%d", userID, productID)
	cachedRecommendations, err := redisClient.Get(ctx, cacheKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var response []RecommendationResponce
	if err := json.Unmarshal([]byte(cachedRecommendations), &response); err != nil {
		return nil, err
	}
	return response, nil
}

func incrementRequestCount(userID int, productID int) {
	requestCountKey := fmt.Sprintf("request_count:%d:%d", userID, productID)
	redisClient.Incr(ctx, requestCountKey)
}

func getRequestCount(userID int, productID int) int64 {
	requestCountKey := fmt.Sprintf("request_count:%d:%d", userID, productID)
	requestCount, _ := redisClient.Get(ctx, requestCountKey).Int64()
	return requestCount
}

func parseRequest(r *http.Request) (RecommendationRequest, error) {
	var req RecommendationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	return req, nil
}

func sendResponse(w http.ResponseWriter, response []RecommendationResponce) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		log.Printf("Error encoding response: %v", err)
	}
}

func getRecommendations(userID int, productID int) ([]Product, error) {
	liked, err := isProductLikedByUser(userID, productID)
	if err != nil {
		return nil, err
	}

	var result []Product
	if liked {
		result, err = getRecommendationsForLikedProduct(productID)
		if err != nil {
			return nil, err
		}
	} else {
		result, err = getRecommendationsForUnlikedProduct(userID)
		if err != nil {
			return nil, err
		}
	}

	// Если количество рекомендаций меньше 3, добавляем топ залайканные продукты
	if len(result) < 3 {
		topLikedProducts, err := getTopLikedProducts()
		if err != nil {
			return nil, err
		}

		// Добавляем недостающие продукты из топа
		for _, product := range topLikedProducts {
			// Проверяем, чтобы не добавлять дубликаты
			if len(result) >= 3 {
				break
			}
			// Проверяем, что продукт не уже в результатах
			if !containsProduct(result, product) {
				result = append(result, product)
			}
		}
	}

	return result, nil
}

func containsProduct(products []Product, product Product) bool {
	for _, p := range products {
		if p.ID == product.ID {
			return true
		}
	}
	return false
}

func isProductLikedByUser(userID int, productID int) (bool, error) {
	var liked bool
	err := db.GetDB().QueryRow("SELECT EXISTS(SELECT 1 FROM likes WHERE user_id = $1 AND product_id = $2)", userID, productID).Scan(&liked)
	return liked, err
}

func getRecommendationsForLikedProduct(productID int) ([]Product, error) {
	category, err := getProductCategory(productID)
	if err != nil {
		return nil, err
	}

	recommendations, err := getTopProductsByCategory(category)
	if err != nil {
		return nil, err
	}

	if len(recommendations) == 0 {
		return getTopLikedProducts()
	}

	return recommendations, nil
}

func getProductCategory(productID int) (string, error) {
	var category string
	err := db.GetDB().QueryRow("SELECT category FROM products WHERE id = $1", productID).Scan(&category)
	return category, err
}

func getTopProductsByCategory(category string) ([]Product, error) {
	rows, err := db.GetDB().Query("SELECT id, category, likes FROM products WHERE category = $1 ORDER BY likes DESC LIMIT 3", category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Category, &p.Likes); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	return products, nil
}

func getRecommendationsForUnlikedProduct(userID int) ([]Product, error) {
	categories, err := getLikedCategoriesByUser(userID)
	if err != nil {
		return nil, err
	}

	if len(categories) == 0 {
		return getTopLikedProducts()
	}

	return getProductsFromLikedCategories(categories)
}

func getLikedCategoriesByUser(userID int) ([]string, error) {
	categoryRows, err := db.GetDB().Query("SELECT category FROM products p JOIN likes l ON p.id = l.product_id WHERE l.user_id = $1 GROUP BY category ORDER BY COUNT(l.product_id) DESC", userID)

	if err != nil {
		return nil, err
	}
	defer categoryRows.Close()

	var categories []string
	for categoryRows.Next() {
		var category string
		if err := categoryRows.Scan(&category); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}

	return categories, nil
}

func getProductsFromLikedCategories(categories []string) ([]Product, error) {
	var recommendations []Product

	for _, category := range categories {
		rows, err := db.GetDB().Query("SELECT id, category, likes FROM products WHERE category = $1 ORDER BY likes DESC LIMIT 3", category)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var p Product
			if err := rows.Scan(&p.ID, &p.Category, &p.Likes); err != nil {
				return nil, err
			}
			recommendations = append(recommendations, p)
			if len(recommendations) >= 3 { // Ограничиваем до 3 рекомендаций
				return recommendations[:3], nil
			}
		}
	}

	// Если все равно меньше 3 рекомендаций:
	if len(recommendations) > 3 {
		return recommendations[:3], nil
	}
	return recommendations, nil
}

func getTopLikedProducts() ([]Product, error) {
	var products []Product

	rows, err := db.GetDB().Query("SELECT id, category, likes FROM products ORDER BY likes DESC LIMIT 3")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Category, &p.Likes); err != nil {
			return nil, err
		}
		products = append(products, p)
	}

	return products, nil
}

func top3(w http.ResponseWriter, r *http.Request) {
	recommendations, err := getTopLikedProducts()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting recommendations: %v", err), http.StatusInternalServerError)
		return
	}

	top3 := make([]RecommendationResponce, 3)

	for i, id := range recommendations {
		top3[i].ProductID = id.ID
	}
	if err := json.NewEncoder(w).Encode(top3); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

func InitializeRoutes() {
	db.Connect()
	http.HandleFunc("/recommendations/", recommend)
	http.HandleFunc("/recommendations/top3", top3)
}

/*

KAFKA

*/

func InitKafka() {
	consumer, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": os.Getenv("KAFKA_BROKER"),
		"group.id":          "recommendations",
		"auto.offset.reset": "earliest",
	})
	if err != nil {
		log.Fatalf("Ошибка создания консьюмера: %v", err)
	}
	defer consumer.Close()

	// Подписка на топик
	consumer.SubscribeTopics([]string{"product_updates"}, nil)

	// Чтение сообщений
	kafkaLoop(consumer)

}

func kafkaLoop(consumer *kafka.Consumer) {
	for {
		msg, err := consumer.ReadMessage(-1)
		if err == nil {
			var event KafkaMessage
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Error unmarshalling message: %s", err)
				continue
			}
			processKafkaMessage(event)
		} else {
			log.Printf("Error while consuming message: %s", err)
		}

	}
}

func processKafkaMessage(event KafkaMessage) {
	switch event.Action {
	case "like":
		processLike(event)
	case "unlike":
		processUnlike(event)
	}
}

// Функция для обработки лайков
func processLike(event KafkaMessage) {
	query := `INSERT INTO likes (user_id, product_id) VALUES ($1, $2)`
	if _, err := db.GetDB().Exec(query, event.UserID, event.ProductID); err != nil {
		log.Printf("Error inserting like into database: %v", err)
		return
	}
	productId, _ := strconv.Atoi(event.ProductID)
	if isRecommendationInDB(event.UserID, productId) {
		updateRecommendationInDB(event.UserID, productId)
	}

	log.Printf("User %d liked product %s", event.UserID, event.ProductID)
}

// Функция для обработки анлайков
func processUnlike(event KafkaMessage) {
	query := `DELETE FROM likes WHERE user_id = $1 AND product_id = $2`
	if _, err := db.GetDB().Exec(query, event.UserID, event.ProductID); err != nil {
		log.Printf("Error deleting like from database: %v", err)
		return
	}
	productId, _ := strconv.Atoi(event.ProductID)
	if isRecommendationInDB(event.UserID, productId) {
		updateRecommendationInDB(event.UserID, productId)
	}

	log.Printf("User %d unliked product %s", event.UserID, event.ProductID)
}

func addRecommendationToBD(userID int, productID int, recommendations []RecommendationResponce) {
	query := `INSERT INTO recommendations (user_id, product_id, recommendation1, recommendation2, recommendation3) VALUES ($1, $2, $3, $4, $5)`
	if _, err := db.GetDB().Exec(query, userID, productID, recommendations[0].ProductID, recommendations[1].ProductID, recommendations[2].ProductID); err != nil {
		log.Printf("Error adding recommendation to database: %v", err)
		return
	}
}

func isRecommendationInDB(userID int, productID int) bool {
	var exists bool
	db.GetDB().QueryRow("SELECT EXISTS(SELECT 1 FROM recommendations WHERE user_id = $1 AND product_id = $2)", userID, productID).Scan(&exists)
	return exists
}

func getRecommendationFromDB(userID int, productID int) []RecommendationResponce {
	recommendations := make([]RecommendationResponce, 3)
	err := db.GetDB().QueryRow("SELECT recommendation1, recommendation2, recommendation3 FROM recommendations WHERE user_id = $1 AND product_id = $2", userID, productID).Scan(&recommendations[0].ProductID, &recommendations[1].ProductID, &recommendations[2].ProductID)
	if err == sql.ErrNoRows {
		log.Printf("Error getting recommendation from database: %v", err)
		return nil
	}
	return recommendations
}

// func isRecommendationInRedis(userID int, productID int) bool {}

func updateRecommendationInDB(userID int, productID int) {
	recommendations, err := getRecommendations(userID, productID)
	if err != nil {
		log.Printf("Error getting recommendation from database: %v", err)
		return
	}
	_, err = db.GetDB().Exec("UPDATE recommendations SET recommendation1 = $3, recommendation2 = $4, recommendation3 = $5 WHERE user_id = $1 AND product_id = $2",
		userID, productID, recommendations[0].ID, recommendations[1].ID, recommendations[2].ID)

	if err != nil {
		log.Printf("Error updating recommendation from database: %v", err)
		return
	}
}
