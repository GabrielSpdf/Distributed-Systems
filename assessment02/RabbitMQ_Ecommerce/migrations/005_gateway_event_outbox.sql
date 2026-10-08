BEGIN;

CREATE TABLE IF NOT EXISTS gateway.event_outbox (
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES gateway.orders(id),
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    envelope JSONB,
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
    UNIQUE (order_id, event_type),
    CHECK (status <> 'PUBLICADO' OR envelope IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_event_outbox_pending
    ON gateway.event_outbox(next_attempt_at, created_at)
    WHERE status = 'PENDENTE';

COMMIT;