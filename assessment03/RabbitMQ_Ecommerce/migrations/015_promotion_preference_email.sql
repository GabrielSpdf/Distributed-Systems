BEGIN;

-- E-mail informado pelo cliente na inscrição (enunciado: "registrar interesse,
-- informando o e-mail"). Vazio nas linhas antigas: vale o e-mail da conta.
ALTER TABLE gateway.promotion_preferences
    ADD COLUMN IF NOT EXISTS email TEXT;

COMMIT;
