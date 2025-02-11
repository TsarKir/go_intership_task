# Recommendation and Analytics System Documentation

This document outlines the architecture, microservice interactions, technologies used, and setup instructions for the recommendation and analytics system as part of an internship test assignment.

## System Architecture and Microservice Interactions

### System Architecture

The system comprises several microservices, each performing specific functions. The main components are:

1.  **User Service**: Manages users, including registration, authentication using JWT, and profile updates.
2.  **Product Service**: Manages products, including creation, updates, deletion, and information retrieval.
3.  **Recommendation Service**: Generates recommendations for users based on their preferences and like history. The implementation follows these principles:
    *   If a user has no likes yet, the top 3 most liked products in the system are recommended.
    *   If a user likes a product on whose page they are, the top 3 most liked products in the same category are displayed.
    *   If a category has fewer than 3 products, all available products are shown, supplemented by the most liked products system-wide.
4.  **Analytics Service**: Collects data on user and product activities and stores it in a database for subsequent analysis.
5.  **Kafka**: Used for asynchronous communication between microservices via two topics: `user_updates` and `product_updates`.
6.  **PostgreSQL**: Database for storing user, product, and recommendation information. Each microservice has its own database, but they are hosted in a single container.
7.  **Redis**: Cache for storing frequently accessed data. In this implementation, recommendations are cached. If a recommendation for a user for a specific product is requested more than 5 times, it is cached and retrieved from there on subsequent requests.
8.  **Nginx**: Reverse proxy server for routing requests to the appropriate microservices.

### Microservice Interactions

1.  **User Service**:
    *   Interacts with Kafka to send messages about user activities (registration, profile updates).
    *   Interacts with PostgreSQL to store user information.
2.  **Product Service**:
    *   Interacts with Kafka to send messages about product activities (creation, updates, deletion, likes).
    *   Interacts with PostgreSQL to store product information.
3.  **Recommendation Service**:
    *   Interacts with Kafka to receive messages about user and product activities.
    *   Interacts with PostgreSQL to store and retrieve recommendations.
    *   Interacts with Redis to cache recommendations.
4.  **Analytics Service**:
    *   Interacts with Kafka to receive messages about user and product activities.
    *   Interacts with PostgreSQL to store analytics data.
5.  **Kafka**:
    *   Provides asynchronous communication between microservices.
    *   Used for sending and receiving messages about user and product activities.
6.  **PostgreSQL**:
    *   Stores user, product, and recommendation information.
7.  **Redis**:
    *   Caches frequently accessed data to improve access speed.
8.  **Nginx**:
    *   Routes incoming requests to the appropriate microservices.

## Technologies Used and Configuration

### Technologies

1.  **Go**: Programming language for writing microservices.
2.  **Kafka**: System for asynchronous communication between microservices.
3.  **PostgreSQL**: Relational database for storing information.
4.  **Redis**: Cache for storing frequently accessed data.
5.  **Nginx**: Reverse proxy server for request routing.
6.  **Docker**: Containerization of microservices for simplified deployment and management.
7.  **JWT**: JSON Web Tokens for user authentication.

### Configuration

1.  **Kafka**:
    *   **Broker**: `kafka:9092`
    *   **Topics**: `user_updates`, `product_updates`
2.  **PostgreSQL**:
    *   **User**: `postgres`
    *   **Password**: `1`
    *   **Databases**: `users_db`, `products_db`, `recommends_db`, `analytics_db`
3.  **Redis**:
    *   **URL**: `redis:6379`
4.  **JWT**:
    *   **Secret Key**: `secret`
5.  **Admin Credentials**
    *   **email**: `admin@site.com`
    *   **password**: `pass`

## Startup Instructions

### Prerequisites

1.  Install Docker and Docker Compose.
2.  Ensure you have internet access to download Docker images.

### Startup Steps

1.  **Clone the repository**:

`git clone <repository_url>`

`cd <repository_directory>`

3.  **Build and run the containers**:

`docker-compose up --build -d`

3.  **Check the container status**:

`docker-compose ps`

4.  **Accessing Services**:
 *   **Homepage**: `http://localhost:8080`
 *   **User Service**: `http://localhost:9999`
 *   **Product Service**: `http://localhost:7777`
 *   **Recommendation Service**: `http://localhost:6666`
 *   **Analytics Service**: `http://localhost:5555`

### Shutdown

To stop and remove the containers, run:

`docker-compose down`

This documentation covers the main aspects of the system architecture, microservice interactions, technologies used, and startup instructions.
