package phandler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"products/db"
	"strconv"
	"text/template"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/dgrijalva/jwt-go"
)

var userUpdateTopic = "product_updates"

var producer *kafka.Producer

var jwtSecret = []byte("secret")

type Recommendation struct {
	Name string `json:"name"`
	Url  string `json:"url"`
}

type ResFromRecommendation struct {
	ID int `json:"id"`
}

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	Category    string `json:"category"`
	Likes       int    `json:"likes"`
}
type EditedProduct struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       string `json:"price"`
	Category    string `json:"category"`
	Likes       int    `json:"likes"`
}

type Like struct {
	UserID    int `json:"user_id"`
	ProductID int `json:"product_id"`
}

type KafkaMessage struct {
	Action             string `json:"action"`
	UserID             int    `json:"user_id"`
	ProductID          string `json:"product_id"`
	ProductCategory    string `json:"product_category"`
	NumberOfLikes      int    `json:"number_of_likes"`
	ProductDescription string `json:"description"`
	ProductName        string `json:"name"`
}

func getFromJWT(str string, w http.ResponseWriter, r *http.Request) interface{} {
	cookie, err := r.Cookie("token")
	if err != nil || cookie == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}

	tokenString := cookie.Value
	claims, err := parseJWT(tokenString)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return false
	}
	return claims[str]
}

// Проверка прав администратора
func isAdmin(w http.ResponseWriter, r *http.Request) bool {
	role := getFromJWT("role", w, r)
	return role == "admin"
}

// Парсинг JWT токена
func parseJWT(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, err
}

// Страница администратора
func adminPage(w http.ResponseWriter, r *http.Request) {
	if isAdmin(w, r) {
		tmpl := template.Must(template.ParseFiles("templates/admin.html"))
		tmpl.Execute(w, nil)
	} else {
		http.Error(w, "Access denied", http.StatusForbidden)
	}
}

// Начальная страница с продуктами
func productsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	top3, _ := getTop3Recommendation()
	responce := fromResToRecs(top3)
	tmpl, err := template.ParseFiles("templates/products.html")
	if err != nil {
		http.Error(w, "Could not load template", http.StatusInternalServerError)
		return
	}
	// Создаем структуру для передачи данных в шаблон
	data := struct {
		Recommendations []Recommendation
	}{
		Recommendations: responce,
	}

	// Выполняем шаблон с данными о продукте и рекомендациями
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Could not execute template", http.StatusInternalServerError)
		return
	}
}

// Получение продукта по ID
func getProduct(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var product Product

	err := db.GetDB().QueryRow("SELECT id, name, description, price, category, likes FROM products WHERE id=$1", id).Scan(&product.ID, &product.Name, &product.Description, &product.Price, &product.Category, &product.Likes)
	if err == sql.ErrNoRows {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	// Проверяем, является ли пользователь администратором
	isAdmin := isAdmin(w, r)

	// Загружаем HTML-шаблон
	tmpl, err := template.ParseFiles("templates/product.html")
	if err != nil {
		http.Error(w, "Could not load template", http.StatusInternalServerError)
		return // Завершаем выполнение функции после отправки ошибки
	}

	userID := int(getFromJWT("id", w, r).(float64))
	recommendations, err := getRecommendations(userID, product.ID)

	if err != nil {
		fmt.Println(err)
		http.Error(w, "Could not receive recommendations", http.StatusInternalServerError)
		return
	}

	// Создаем структуру для передачи данных в шаблон
	data := struct {
		Product         Product
		IsAdmin         bool
		IsLiked         bool
		Recommendations []Recommendation
	}{
		Product:         product,
		IsAdmin:         isAdmin,
		IsLiked:         isLiked(userID, id),
		Recommendations: recommendations,
	}

	// Выполняем шаблон с данными о продукте
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Could not execute template", http.StatusInternalServerError)
		return // Завершаем выполнение функции после отправки ошибки
	}
}

/*


ДОБАВЛЕНИЕ ПРОДУКТА


*/

func addProductPage(w http.ResponseWriter, r *http.Request) {
	if isAdmin(w, r) {
		tmpl := template.Must(template.ParseFiles("templates/add_product.html"))
		tmpl.Execute(w, nil)
	}
}

func addProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	var product EditedProduct
	if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err := db.GetDB().Exec("INSERT INTO products (name, description, price, category, likes) VALUES ($1, $2, $3, $4, $5)",
		product.Name, product.Description, product.Price, product.Category, product.Likes)
	if err != nil {
		http.Error(w, "Could not create product", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"name": product.Name, "description": product.Description, "price": product.Price, "category": product.Category, "likes": product.Likes})
}

/*


УДАЛЕНИЕ ПРОДУКТА


*/

func deleteProduct(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что метод запроса - DELETE
	if r.Method != http.MethodDelete {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing product ID", http.StatusBadRequest)
		return
	}
	// Удаляем товар из базы данных
	result, err := db.GetDB().Exec("DELETE FROM products WHERE id = $1", id)
	if err != nil {
		http.Error(w, "Could not delete product", http.StatusInternalServerError)
		return
	}
	// Проверяем, было ли удалено хоть одно значение
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "Could not check affected rows", http.StatusInternalServerError)
		return
	}
	if rowsAffected == 0 {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}
}

/*


ОБНОВЛЕНИЕ ИНФОРМАЦИИ О ПРОДУКТЕ


*/

func updateProductPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	var product Product
	err := db.GetDB().QueryRow("SELECT id, name, description, price, category, likes FROM products WHERE id=$1", id).Scan(&product.ID, &product.Name, &product.Description, &product.Price, &product.Category, &product.Likes)

	if err == sql.ErrNoRows {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	tmpl, err := template.ParseFiles("templates/product_update.html")
	if err != nil {
		http.Error(w, "Could not load template", http.StatusInternalServerError)
		return
	}

	err = tmpl.Execute(w, struct{ Product Product }{Product: product})
	if err != nil {
		http.Error(w, "Could not execute template", http.StatusInternalServerError)
		return
	}
}

func updateProduct(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что метод запроса - POST
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Получаем ID товара из параметров запроса
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	// Получаем данные из формы
	name := r.FormValue("name")
	description := r.FormValue("description")
	price := r.FormValue("price")
	category := r.FormValue("category")
	likes := r.FormValue("likes")

	// Обновляем информацию о товаре в базе данных
	_, err := db.GetDB().Exec("UPDATE products SET name = $1, description = $2, price = $3, category = $4, likes = $5 WHERE id = $6",
		name, description, price, category, likes, id)

	if err != nil {
		http.Error(w, "Could not update product", http.StatusInternalServerError)
		return
	}

	likes_int, _ := strconv.Atoi(likes)
	userID := int(getFromJWT("id", w, r).(float64))
	msg := KafkaMessage{

		UserID:             userID,
		ProductID:          id,
		Action:             "product info update",
		ProductCategory:    category,
		ProductName:        name,
		ProductDescription: description,
		NumberOfLikes:      likes_int,
	}
	sendToKafka(msg)

	// Успешное обновление
	http.Redirect(w, r, "/products/product?id="+id, http.StatusSeeOther)
}

/*


ЛАЙКИ


*/

func toggleLike(w http.ResponseWriter, r *http.Request) {
	// Получаем токен из куки

	userID := int(getFromJWT("id", w, r).(float64)) // Преобразование user_id в int

	// Получаем product_id из параметров запроса
	productID := r.URL.Query().Get("id")
	if productID == "" {
		http.Error(w, "Missing product ID", http.StatusBadRequest)
		return
	}

	// Проверяем существование лайка
	if isLiked(userID, productID) {
		// Если лайк уже существует, удаляем его
		if err := removeLike(userID, productID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Если лайк не существует, добавляем его
		if err := addLike(userID, productID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

}

func isLiked(userID int, productID string) bool {
	var exists bool
	err := db.GetDB().QueryRow("SELECT EXISTS(SELECT 1 FROM likes WHERE user_id = $1 AND product_id = $2)", userID, productID).Scan(&exists)
	if err == sql.ErrNoRows {
		return false
	}
	return exists
}

func addLike(userID int, productID string) error {
	_, err := db.GetDB().Exec("INSERT INTO likes (user_id, product_id) VALUES ($1, $2)", userID, productID)
	if err != nil {
		return err
	}
	_, err = db.GetDB().Exec("UPDATE products SET likes = likes + 1 WHERE id = $1", productID)
	if err != nil {
		return err
	}
	var name, category, description string
	var likes, id int
	err = db.GetDB().QueryRow("SELECT id, name, category, description, likes FROM products WHERE id = $1", productID).Scan(&id, &name, &category, &description, &likes)
	if err == sql.ErrNoRows {
		return err
	}
	msg := KafkaMessage{

		UserID:             userID,
		ProductID:          productID,
		Action:             "like",
		ProductCategory:    category,
		ProductName:        name,
		ProductDescription: description,
		NumberOfLikes:      likes,
	}
	sendToKafka(msg)
	return nil
}

func removeLike(userID int, productID string) error {
	_, err := db.GetDB().Exec("DELETE FROM likes WHERE user_id = $1 AND product_id = $2", userID, productID)
	if err != nil {
		return err
	}
	_, err = db.GetDB().Exec("UPDATE products SET likes = likes - 1 WHERE id = $1", productID)
	if err != nil {
		return err
	}
	var name, category, description string
	var likes, id int
	err = db.GetDB().QueryRow("SELECT id, name, category, description, likes FROM products WHERE id = $1", productID).Scan(&id, &name, &category, &description, &likes)
	if err == sql.ErrNoRows {
		return err
	}
	msg := KafkaMessage{

		UserID:             userID,
		ProductID:          productID,
		Action:             "unlike",
		ProductCategory:    category,
		ProductName:        name,
		ProductDescription: description,
		NumberOfLikes:      likes,
	}
	sendToKafka(msg)
	return nil
}

// Инициализация маршрутов
func InitializeRoutes() {
	initKafka()
	db.Connect()
	http.HandleFunc("/products/product/", getProduct)                 // Получение продукта по ID
	http.HandleFunc("/products/admin/add", addProductPage)            // Добавление нового продукта (требует админских прав)
	http.HandleFunc("/products/admin", adminPage)                     // Админка
	http.HandleFunc("/products/admin/add/submit", addProduct)         // Post запрос на добавление продукта
	http.HandleFunc("/products/product/delete", deleteProduct)        // delete запрос для удаления продукта
	http.HandleFunc("/products/product/update", updateProductPage)    // Для отображения формы обновления товара
	http.HandleFunc("/products/product/update/submit", updateProduct) // Подтверждаем изменения информации о товаре
	http.HandleFunc("/products/product/like", toggleLike)             // Для обработки обновления товара (POST)
	http.HandleFunc("/products", productsPage)

}

// kafka

func initKafka() {
	var err error
	producer, err = kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": os.Getenv("KAFKA_BROKER")})
	if err != nil {
		log.Fatalf("Не удалось создать продюсер: %s", err)
	}
}

func sendToKafka(msg KafkaMessage) {
	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		log.Fatalf("Ошибка при сериализации в JSON: %v", err)
	}
	producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &userUpdateTopic, Partition: kafka.PartitionAny},
		Value:          []byte(jsonMsg),
	}, nil)
}

// Рекомендации

func getRecommendations(userId int, productId int) ([]Recommendation, error) {
	// Создаем объект для отправки в теле запроса
	requestBody := map[string]int{
		"user_id":    userId,
		"product_id": productId,
	}

	// Преобразуем объект в JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Отправляем POST-запрос
	resp, err := http.Post("http://recommendation-service:6666/recommendations/", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close() // Закрываем тело ответа после завершения работы с ним

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response status: %s", resp.Status)
	}

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result []ResFromRecommendation
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return fromResToRecs(result), nil
}

func getTop3Recommendation() ([]ResFromRecommendation, error) {
	// Выполняем GET запрос к API
	resp, err := http.Get("http://recommendation-service:6666/recommendations/top3")
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close() // Закрываем тело ответа после завершения работы с ним

	// Проверяем статус ответа
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response status: %s", resp.Status)
	}

	// Читаем тело ответа
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Декодируем JSON в структуру
	var recommendations []ResFromRecommendation
	if err := json.Unmarshal(body, &recommendations); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return recommendations, nil
}

func fromResToRecs(data []ResFromRecommendation) []Recommendation {
	res := make([]Recommendation, 3)
	for i, rec := range data {
		err := db.GetDB().QueryRow("SELECT name FROM products WHERE id = $1", rec.ID).Scan(&res[i].Name)
		if err == sql.ErrNoRows {
			return nil
		}
		res[i].Url = fmt.Sprintf("/products/product?id=%d", rec.ID)

	}
	return res
}
