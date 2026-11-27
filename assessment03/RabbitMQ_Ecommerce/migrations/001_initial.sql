CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS gateway;
CREATE SCHEMA IF NOT EXISTS estoque;
CREATE SCHEMA IF NOT EXISTS pagamento;
CREATE SCHEMA IF NOT EXISTS promocoes;

CREATE TABLE IF NOT EXISTS gateway.users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(120) NOT NULL,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gateway.orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_id VARCHAR(32) NOT NULL UNIQUE,
    user_id UUID NOT NULL REFERENCES gateway.users(id),
    status VARCHAR(40) NOT NULL,
    total NUMERIC(12, 2) NOT NULL CHECK (total >= 0),
    checkout_url TEXT,
    is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_user_created
    ON gateway.orders(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS gateway.order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES gateway.orders(id) ON DELETE CASCADE,
    product_id VARCHAR(64) NOT NULL,
    product_name VARCHAR(160) NOT NULL,
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    unit_price NUMERIC(12, 2) NOT NULL CHECK (unit_price >= 0)
);

CREATE TABLE IF NOT EXISTS gateway.order_status_history (
    id BIGSERIAL PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES gateway.orders(id) ON DELETE CASCADE,
    status VARCHAR(40) NOT NULL,
    event_id VARCHAR(80),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (order_id, event_id)
);

CREATE TABLE IF NOT EXISTS estoque.products (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(160) NOT NULL,
    category VARCHAR(100) NOT NULL,
    price NUMERIC(12, 2) NOT NULL CHECK (price >= 0),
    quantity INTEGER NOT NULL CHECK (quantity >= 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_products_category
    ON estoque.products(category);

CREATE TABLE IF NOT EXISTS estoque.stock_reservations (
    order_id VARCHAR(64) PRIMARY KEY,
    status VARCHAR(30) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS estoque.stock_reservation_items (
    order_id VARCHAR(64) NOT NULL REFERENCES estoque.stock_reservations(order_id) ON DELETE CASCADE,
    product_id VARCHAR(64) NOT NULL REFERENCES estoque.products(id),
    quantity INTEGER NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (order_id, product_id)
);

CREATE TABLE IF NOT EXISTS pagamento.payment_charges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id VARCHAR(64) NOT NULL UNIQUE,
    amount NUMERIC(12, 2) NOT NULL CHECK (amount >= 0),
    status VARCHAR(30) NOT NULL,
    checkout_url TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS pagamento.webhook_events (
    id BIGSERIAL PRIMARY KEY,
    external_event_id VARCHAR(100) NOT NULL UNIQUE,
    charge_id UUID NOT NULL REFERENCES pagamento.payment_charges(id),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS promocoes.promotion_interests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    email VARCHAR(255) NOT NULL,
    category VARCHAR(100) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, category)
);

CREATE INDEX IF NOT EXISTS idx_promotion_interests_category_active
    ON promocoes.promotion_interests(category, active);

CREATE TABLE IF NOT EXISTS promocoes.email_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL,
    email VARCHAR(255) NOT NULL,
    category VARCHAR(100) NOT NULL,
    provider_message_id VARCHAR(255),
    status VARCHAR(30) NOT NULL,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO estoque.products (id, name, category, price, quantity)
VALUES
    ('PROD-001', 'Arroz 5 kg', 'alimentos', 29.90, 30),
    ('PROD-002', 'Detergente', 'limpeza', 4.90, 50),
    ('PROD-003', 'Fone Bluetooth', 'eletronicos', 119.90, 12)
ON CONFLICT (id) DO NOTHING;

