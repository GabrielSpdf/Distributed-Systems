BEGIN;

CREATE SCHEMA IF NOT EXISTS entrega;

-- Uma entrega por pedido. O envelope assinado de pedido.enviado fica gravado
-- como texto (a assinatura depende dos bytes exatos) para ser reenviado sem
-- mudanças; published_at só é preenchido após a confirmação do RabbitMQ.
CREATE TABLE IF NOT EXISTS entrega.shipments (
    order_id VARCHAR(64) PRIMARY KEY,
    customer_id TEXT NOT NULL,
    charge_id TEXT NOT NULL,
    invoice_id TEXT NOT NULL UNIQUE,
    tracking_code TEXT NOT NULL UNIQUE,
    shipped_at TIMESTAMPTZ NOT NULL,
    envelope TEXT NOT NULL,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS shipments_pending_idx
    ON entrega.shipments (created_at)
    WHERE published_at IS NULL;

COMMIT;
