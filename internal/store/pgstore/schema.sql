-- Idempotent: Migrate runs it on every startup, from every replica.
-- seq preserves creation order for List (UUIDs carry no order, SPEC D2).
CREATE TABLE IF NOT EXISTS todos (
    id        uuid    PRIMARY KEY,
    title     text    NOT NULL,
    completed boolean NOT NULL,
    seq       bigint  GENERATED ALWAYS AS IDENTITY UNIQUE
);
