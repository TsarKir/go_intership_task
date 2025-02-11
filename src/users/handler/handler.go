package handler

// TODO: добавить обновление своего профиля

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"users/db"

	kafka "github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/dgrijalva/jwt-go"
	"golang.org/x/crypto/bcrypt"
)

type UserReg struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Pass  string `json:"pass"`
}

type UserUpdate struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Pass  string `json:"-"`
	Role  string `json:"role"`
}

type UserLog struct {
	Email string `json:"email"`
	Pass  string `json:"pass"`
}

type KafkaMessage struct {
	Action string `json:"action"`
	UserID int    `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

var userUpdateTopic = "user_updates"

var producer *kafka.Producer

var jwtSecret = []byte("secret")

// Home page handler
func homePage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, nil)
}

// Registration page handler
func registrationPage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/registration.html"))
	tmpl.Execute(w, nil)
}

// Login page handler
func loginPage(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, nil)
}

// Create a new user
func createUser(w http.ResponseWriter, r *http.Request) {
	if !isPostRequest(w, r) {
		return
	}

	var user UserReg
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if checkIfExist(user.Email) {
		http.Error(w, "User already exists", http.StatusConflict)
		return
	}

	hashedPassword, err := hashPassword(user.Pass)
	if err != nil {
		http.Error(w, "Could not hash password", http.StatusInternalServerError)
		return
	}
	user.Pass = hashedPassword

	var newUserID int
	err = db.GetDB().QueryRow("INSERT INTO users (name, email, pass, role) VALUES ($1, $2, $3, $4) RETURNING id",
		user.Name, user.Email, user.Pass, "user").Scan(&newUserID)
	if err != nil {
		http.Error(w, "Could not create user", http.StatusInternalServerError)
		return
	}
	// Отправляем сообщение в кафка
	msg := KafkaMessage{

		UserID: newUserID,
		Action: "new user",
		Name:   user.Name,
		Email:  user.Email,
	}
	sendToKafka(msg)
	fmt.Println(msg)

	json.NewEncoder(w).Encode(map[string]interface{}{"id": newUserID, "name": user.Name, "email": user.Email})
}

// Check if user already exists in the database
func checkIfExist(email string) bool {
	var exists bool
	err := db.GetDB().QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)", email).Scan(&exists)
	if err != nil {
		return false // В случае ошибки считаем пользователя не существующим.
	}
	return exists
}

// Hash password using bcrypt
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// Authenticate user and generate JWT token
func login(w http.ResponseWriter, r *http.Request) {
	if !isPostRequest(w, r) {
		return
	}

	var loginUser UserLog // Используем структуру для логина
	if err := json.NewDecoder(r.Body).Decode(&loginUser); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var user User
	err := db.GetDB().QueryRow("SELECT id, name, pass, role FROM users WHERE email=$1", loginUser.Email).Scan(&user.ID, &user.Name, &user.Pass, &user.Role)
	if err == sql.ErrNoRows {
		http.Error(w, "Пользователь не найден", http.StatusNotFound)
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Pass), []byte(loginUser.Pass))
	if err != nil {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	token, err := generateJWT(user)
	if err != nil {
		http.Error(w, "Could not generate token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
	})

	json.NewEncoder(w).Encode(map[string]string{"message": "Login successful"})
}

// Generate JWT token for the user
func generateJWT(user User) (string, error) {
	claims := jwt.MapClaims{
		"id":   user.ID,
		"name": user.Name,
		"exp":  time.Now().Add(time.Hour * 24).Unix(),
		"role": user.Role,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// Get user information by ID from the database
func getUser(w http.ResponseWriter, r *http.Request) {
	// Получаем ID пользователя из параметров запроса
	id := r.URL.Query().Get("id")
	var user User

	// Запрашиваем данные пользователя из базы данных
	err := db.GetDB().QueryRow("SELECT id, name, email, role FROM users WHERE id=$1", id).Scan(&user.ID, &user.Name, &user.Email, &user.Role)
	if err == sql.ErrNoRows {
		http.Error(w, "Пользователь не найден", http.StatusNotFound)
	}

	// Рендерим HTML-шаблон с данными пользователя
	tmpl := template.Must(template.ParseFiles("templates/user.html"))
	if err := tmpl.Execute(w, user); err != nil {
		http.Error(w, "Ошибка при рендеринге шаблона", http.StatusInternalServerError)
	}
}

// Обработчик для страницы редактирования профиля пользователя
func editUserPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var user User

	// Получаем данные пользователя из базы данных
	err := db.GetDB().QueryRow("SELECT id, name, email, role FROM users WHERE id=$1", id).Scan(&user.ID, &user.Name, &user.Email, &user.Role)
	if err == sql.ErrNoRows {
		http.Error(w, "Пользователь не найден", http.StatusNotFound)
	}

	// Рендерим HTML-шаблон с данными для редактирования
	tmpl := template.Must(template.ParseFiles("templates/user_edit.html"))
	if err := tmpl.Execute(w, user); err != nil {
		http.Error(w, "Ошибка при рендеринге шаблона", http.StatusInternalServerError)
	}
}

func editUser(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что метод запроса - POST
	if r.Method != http.MethodPost {
		http.Error(w, "Метод не разрешен", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing user ID", http.StatusBadRequest)
		return
	}

	// Получаем данные из формы
	name := r.FormValue("name")
	email := r.FormValue("email")

	// Обновляем данные пользователя в базе данных
	_, err := db.GetDB().Exec("UPDATE users SET name = $1, email = $2 WHERE id = $3",
		name, email, id)

	if err != nil {
		http.Error(w, "Ошибка при обновлении данных пользователя", http.StatusInternalServerError)
		return
	}
	// Отправляем сообщение в кафка
	ID, _ := strconv.Atoi(id)
	msg := KafkaMessage{

		UserID: ID,
		Action: "update user info",
		Name:   name,
		Email:  email,
	}
	fmt.Println(msg)
	sendToKafka(msg)
	// Отправляем успешный ответ
	http.Redirect(w, r, "/users/user?id="+id, http.StatusSeeOther)
}

// Utility function to check if the request method is POST and return an error if not.
func isPostRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

// Initialize routes

func InitializeRoutes() {
	db.Connect()
	initKafka()
	http.HandleFunc("/", homePage)
	http.HandleFunc("/users/registration", registrationPage)
	http.HandleFunc("/users/login", loginPage)

	http.HandleFunc("/users/registration/create", createUser) // POST for creating a new user
	http.HandleFunc("/users/login/submit", login)             // POST for logging in a user

	http.HandleFunc("/users/user/", getUser) // GET for getting a user by ID from the database.

	http.HandleFunc("/users/edit/", editUserPage)
	http.HandleFunc("/users/edit/submit", editUser)
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
