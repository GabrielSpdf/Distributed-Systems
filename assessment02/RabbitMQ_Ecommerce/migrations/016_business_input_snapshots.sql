BEGIN;

-- Original business input, stored atomically with each effect.
-- NULL preserves legacy rows; do not invent missing historical inputs.
ALTER TABLE estoque.stock_reservations
    ADD COLUMN IF NOT EXISTS request_payload JSONB;

ALTER TABLE entrega.shipments
    ADD COLUMN IF NOT EXISTS payment_payload JSONB;

COMMIT;
