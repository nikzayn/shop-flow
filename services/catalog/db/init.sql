CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    sku TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    price_cents INTEGER NOT NULL,
    stock INTEGER NOT NULL CHECK (stock >= 0)
);