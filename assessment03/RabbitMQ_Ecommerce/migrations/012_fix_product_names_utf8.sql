BEGIN;

-- ASCII-only SQL: Unicode escapes survive PowerShell pipes with legacy encodings.
-- Restore known corrupted names only; never change prices or stock quantities.
UPDATE estoque.products AS product
SET name = corrected.name
FROM (VALUES
    ('PROD-004', U&'Feij\00E3o 1 kg'),
    ('PROD-005', U&'Caf\00E9 500 g'),
    ('PROD-006', U&'A\00E7\00FAcar 1 kg'),
    ('PROD-007', U&'Macarr\00E3o 500 g'),
    ('PROD-009', U&'Sab\00E3o em p\00F3 1 kg'),
    ('PROD-011', U&'\00C1gua sanit\00E1ria 1 L')
) AS corrected(id, name)
WHERE product.id = corrected.id
  AND (
      product.name LIKE '%?%'
      OR strpos(product.name, chr(65533)) > 0
  );

COMMIT;
