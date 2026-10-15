BEGIN;

CREATE TABLE IF NOT EXISTS promocoes.email_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    promotion_event_id UUID NOT NULL,
    user_id UUID NOT NULL,
    category VARCHAR(100) NOT NULL
        CHECK (category IN ('alimentos', 'limpeza', 'eletronicos')),

    -- Conteúdo fixo para reutilizar nas tentativas.
    from_address TEXT NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    subject TEXT NOT NULL CHECK (length(trim(subject)) > 0),
    text_body TEXT NOT NULL DEFAULT '',
    html_body TEXT NOT NULL DEFAULT '',
    idempotency_key VARCHAR(256) NOT NULL UNIQUE
        CHECK (length(trim(idempotency_key)) > 0),

    status VARCHAR(30) NOT NULL DEFAULT 'PENDING'
        CHECK (status IN (
            'PENDING',
            'SENT',
            'FAILED',
            'CANCELLED',
            'PENDING_REVIEW'
        )),
    attempt_count INTEGER NOT NULL DEFAULT 0
        CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    first_attempt_at TIMESTAMPTZ,
    provider_message_id VARCHAR(255),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at TIMESTAMPTZ,

    UNIQUE (promotion_event_id, user_id),
    CHECK (
        length(trim(text_body)) > 0
        OR length(trim(html_body)) > 0
    ),
    CHECK (
        status <> 'SENT'
        OR (
            provider_message_id IS NOT NULL
            AND length(trim(provider_message_id)) > 0
            AND sent_at IS NOT NULL
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_email_jobs_pending
    ON promocoes.email_jobs(next_attempt_at, created_at)
    WHERE status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_email_jobs_user_category
    ON promocoes.email_jobs(user_id, category);

ALTER TABLE promocoes.email_logs
    ADD COLUMN IF NOT EXISTS job_id UUID
        REFERENCES promocoes.email_jobs(id),
    ADD COLUMN IF NOT EXISTS attempt_number INTEGER
        CHECK (attempt_number > 0);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_logs_job_attempt
    ON promocoes.email_logs(job_id, attempt_number);

COMMIT;