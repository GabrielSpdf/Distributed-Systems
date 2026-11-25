BEGIN;

-- Escolhas do cliente, consultadas pelo Gateway.
CREATE TABLE IF NOT EXISTS gateway.promotion_preferences (
    user_id UUID NOT NULL REFERENCES gateway.users(id),
    category VARCHAR(100) NOT NULL
        CHECK (category IN ('alimentos', 'limpeza', 'eletronicos')),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, category)
);

-- Mudanças aguardando publicação para o MS Promoções.
CREATE TABLE IF NOT EXISTS gateway.promotion_outbox (
    sequence_id BIGSERIAL PRIMARY KEY,
    event_id UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES gateway.users(id),
    event_type VARCHAR(100) NOT NULL DEFAULT 'interesse.promocao'
        CHECK (event_type = 'interesse.promocao'),
    payload JSONB NOT NULL
        CHECK (jsonb_typeof(payload) = 'object'),
    envelope TEXT,
    status VARCHAR(30) NOT NULL DEFAULT 'PENDENTE'
        CHECK (status IN (
            'PENDENTE',
            'PUBLICADO',
            'PENDENTE_REVISAO'
        )),
    attempt_count INTEGER NOT NULL DEFAULT 0
        CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    CHECK (
        status <> 'PUBLICADO'
        OR (envelope IS NOT NULL AND published_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_promotion_outbox_pending
    ON gateway.promotion_outbox(next_attempt_at, sequence_id)
    WHERE status = 'PENDENTE';

CREATE INDEX IF NOT EXISTS idx_promotion_outbox_user_sequence
    ON gateway.promotion_outbox(user_id, sequence_id);

COMMIT;