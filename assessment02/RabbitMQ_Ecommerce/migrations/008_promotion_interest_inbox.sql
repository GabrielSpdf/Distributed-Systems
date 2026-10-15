BEGIN;

-- Registro dos eventos aplicados pelo MS Promoções.
CREATE TABLE IF NOT EXISTS promocoes.interest_inbox (
    event_id UUID PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL
        CHECK (event_type = 'interesse.promocao'),
    user_id UUID NOT NULL,
    envelope TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_interest_inbox_user_received
    ON promocoes.interest_inbox(user_id, received_at);

COMMIT;