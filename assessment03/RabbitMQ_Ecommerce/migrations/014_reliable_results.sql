BEGIN;

-- Resultado do Estoque guardado para ser reenviado sem mudanças: as faltas
-- originais (estoque.indisponivel nunca é recalculado) e o envelope assinado,
-- publicado com confirmação do RabbitMQ.
ALTER TABLE estoque.stock_reservations
    ADD COLUMN IF NOT EXISTS shortfalls TEXT,
    ADD COLUMN IF NOT EXISTS result_envelope TEXT,
    ADD COLUMN IF NOT EXISTS result_published_at TIMESTAMPTZ;

-- Envelopes assinados de checkout e de resultado do Pagamento.
ALTER TABLE pagamento.payment_charges
    ADD COLUMN IF NOT EXISTS checkout_envelope TEXT,
    ADD COLUMN IF NOT EXISTS result_envelope TEXT;

COMMIT;
