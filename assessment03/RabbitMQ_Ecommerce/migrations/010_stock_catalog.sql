BEGIN;

INSERT INTO estoque.products (id, name, category, price, quantity)
VALUES
    ('PROD-004', 'Feijão 1 kg', 'alimentos', 8.90, 25),
    ('PROD-005', 'Café 500 g', 'alimentos', 24.90, 18),
    ('PROD-006', 'Açúcar 1 kg', 'alimentos', 4.50, 40),
    ('PROD-007', 'Macarrão 500 g', 'alimentos', 5.90, 35),
    ('PROD-008', 'Leite 1 L', 'alimentos', 6.20, 20),
    ('PROD-009', 'Sabão em pó 1 kg', 'limpeza', 14.90, 15),
    ('PROD-010', 'Desinfetante 2 L', 'limpeza', 9.90, 22),
    ('PROD-011', 'Água sanitária 1 L', 'limpeza', 4.20, 28),
    ('PROD-012', 'Mouse sem fio', 'eletronicos', 59.90, 10),
    ('PROD-013', 'Teclado USB', 'eletronicos', 79.90, 8),
    ('PROD-014', 'Carregador USB-C', 'eletronicos', 49.90, 0)
ON CONFLICT (id) DO NOTHING;

COMMIT;
