-- Создание баз данных
CREATE DATABASE users_db;
CREATE DATABASE products_db;
CREATE DATABASE recommends_db;
CREATE DATABASE analytics_db;


-- Предоставление прав пользователю на базы данных
GRANT ALL PRIVILEGES ON DATABASE users_db TO postgres;
GRANT ALL PRIVILEGES ON DATABASE products_db TO postgres;
GRANT ALL PRIVILEGES ON DATABASE recommends_db TO postgres;
GRANT ALL PRIVILEGES ON DATABASE analytics_db TO postgres;

\connect users_db;

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100) UNIQUE NOT NULL,
    pass VARCHAR(255),
    role VARCHAR(50)
);

INSERT INTO users (name, email, pass, role) VALUES
('admin', 'admin@site.com', '$2a$10$K5VW.PoACy52RYwBABqjPu.RECC7Ln5BS4xO9pMnPUi8atgIDtkue', 'admin');

\connect products_db;

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    description VARCHAR(100),
    price VARCHAR(50),
    category VARCHAR(50),
    likes INT
);

INSERT INTO products (name, description, price, category, likes) VALUES
('Продукт 1', 'cool product 1', '10', 'c1', 10),
('Продукт 2', 'cool product 2', '20', 'c2', 20),
('Продукт 3', 'cool product 3', '30', 'c3', 30);

CREATE TABLE likes (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    product_id INT NOT NULL,
    CONSTRAINT unique_like UNIQUE (user_id, product_id)
);

\connect recommends_db;

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    category VARCHAR(255) NOT NULL,
    likes INT NOT NULL 
);

CREATE TABLE likes (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL,
    product_id INT NOT NULL,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
);

CREATE TABLE recommendations (
    user_id INT,
    product_id INT,
    recommendation1 INT,
    recommendation2 INT,
    recommendation3 INT,
    PRIMARY KEY (user_id, product_id)
);

INSERT INTO products (id, category, likes) VALUES
(3, 'c3', 30),
(2, 'c2', 22),
(4, 'c4', 0),
(1, 'c1', 9);

\connect analytics_db;

CREATE TABLE user_actions (
    id SERIAL PRIMARY KEY,
    user_id INT,
    action VARCHAR(100),
    name VARCHAR(100),
    email VARCHAR(100) UNIQUE NOT NULL
);

CREATE TABLE product_actions (
    id SERIAL PRIMARY KEY,
    product_id INT,
    user_id INT,
    name VARCHAR(100),
    description VARCHAR(100),
    action VARCHAR(50),
    category VARCHAR(50),
    likes INT
);
