BEGIN;

ALTER TABLE gateway.event_outbox
    ALTER COLUMN envelope TYPE TEXT
    USING envelope::text;

COMMIT;