BEGIN;

CREATE TABLE IF NOT EXISTS gateway.event_inbox (
    event_id VARCHAR(80) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    order_id VARCHAR(32) NOT NULL,
    customer_id UUID NOT NULL,
    envelope JSONB NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'RECEBIDO'
        CHECK (status IN (
            'RECEBIDO',
            'PROCESSADO',
            'REJEITADO',
            'PENDENTE_REVISAO'
        )),
    attempt_count INTEGER NOT NULL DEFAULT 0
        CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_event_inbox_pending
    ON gateway.event_inbox(next_attempt_at, received_at)
    WHERE status = 'RECEBIDO';

COMMIT;