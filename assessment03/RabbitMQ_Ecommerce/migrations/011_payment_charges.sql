BEGIN;

-- O id da cobrança passa a ser o charge_id devolvido pelo Mock de Pagamento.
ALTER TABLE pagamento.payment_charges
    ADD COLUMN IF NOT EXISTS customer_id TEXT,
    ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'BRL',
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS processed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reason TEXT,
    ADD COLUMN IF NOT EXISTS checkout_published_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS result_published_at TIMESTAMPTZ;

COMMIT;
