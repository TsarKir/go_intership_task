# Используем официальный образ Golang
FROM golang:1.23 AS builder

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем файлы проекта в контейнер
COPY . .

# Устанавливаем зависимости
RUN go mod download

# Сборка приложения
RUN go build -o main .

# Запускаем приложение
CMD ["./main"]

EXPOSE 5555
