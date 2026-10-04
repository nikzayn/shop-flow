-- Catalog schema and seed data. Postgres runs this once, on first start,
-- when mounted into /docker-entrypoint-initdb.d/.

CREATE TABLE products (
    id          BIGSERIAL PRIMARY KEY,
    sku         TEXT UNIQUE NOT NULL,
    name        TEXT NOT NULL,
    price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
    stock       INTEGER NOT NULL CHECK (stock >= 0)
);

INSERT INTO products (sku, name, price_cents, stock) VALUES
    ('SHOE-001',    'Trail Running Shoe',            8999,  120),
    ('SHOE-002',    'Everyday Sneaker',              5499,  300),
    ('TSHIRT-001',  'Organic Cotton T-Shirt',        1999,  800),
    ('TSHIRT-002',  'Performance Tee',               2499,  450),
    ('HOODIE-001',  'Zip Hoodie',                    4999,  200),
    ('JEANS-001',   'Slim Fit Jeans',                5999,  250),
    ('JACKET-001',  'Packable Rain Jacket',          7999,   90),
    ('BAG-001',     'Commuter Backpack',             6499,  150),
    ('BOTTLE-001',  'Insulated Water Bottle',        2499,  600),
    ('CAP-001',     'Baseball Cap',                  1499,  400),
    ('SOCKS-001',   'Running Socks (3-pack)',        1299, 1000),
    ('WATCH-001',   'Fitness Tracker Watch',        12999,   75),
    ('EARBUD-001',  'Wireless Earbuds',              9999,  110),
    ('MAT-001',     'Yoga Mat',                      3499,  220),
    ('LAMP-001',    'LED Desk Lamp',                 3999,  130),
    ('MUG-001',     'Ceramic Coffee Mug',             999,  700),
    ('KETTLE-001',  'Electric Kettle',               4499,   80),
    -- Flash-sale items: low stock on purpose, for the oversell test in checkout.
    ('PHONE-001',   'Smartphone (Mega Sale)',       49999,   10),
    ('CONSOLE-001', 'Game Console (Mega Sale)',     39999,    5),
    -- Sold out: the catalog must still show it, with stock 0.
    ('TV-001',      '55" 4K TV',                    59999,    0);
