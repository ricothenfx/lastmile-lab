-- 001_phase2_orders.sql — skema pipeline order (Fase 2).
-- Idempoten: aman dijalankan berulang oleh job db-migrate.

CREATE TABLE IF NOT EXISTS orders (
    id              TEXT PRIMARY KEY,
    idempotency_key TEXT,
    pickup_lat      DOUBLE PRECISION NOT NULL,
    pickup_lon      DOUBLE PRECISION NOT NULL,
    dropoff_lat     DOUBLE PRECISION NOT NULL,
    dropoff_lon     DOUBLE PRECISION NOT NULL,
    status          TEXT NOT NULL DEFAULT 'received',
    -- received | assigned | publish_failed
    created_ms      BIGINT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created_ms ON orders(created_ms);

CREATE TABLE IF NOT EXISTS order_events (
    id         BIGSERIAL PRIMARY KEY,
    order_id   TEXT NOT NULL,
    event_type TEXT NOT NULL,
    -- ingested (order-ingestion) | assigned (dispatch-consumer)
    payload    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- unique per (order, tipe) → replay at-least-once tidak menduplikasi event.
CREATE UNIQUE INDEX IF NOT EXISTS uq_order_events_order_type
    ON order_events(order_id, event_type);
CREATE INDEX IF NOT EXISTS idx_order_events_order ON order_events(order_id);
