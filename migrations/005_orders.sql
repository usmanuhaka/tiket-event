-- Tabel pesanan dan tiket (BR-06 s.d. BR-12).
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

-- Status pesanan:
--   HELD      tiket ditahan 15 menit, kuota sudah berkurang (BR-09)
--   PAID      sudah dibayar (BR-11)
--   EXPIRED   tidak dibayar dalam 15 menit, kuota dikembalikan (BR-10)
--   CANCELLED pesanan HELD dibatalkan karena event dibatalkan (BR-20)
CREATE TABLE IF NOT EXISTS orders (
    id             BIGINT    NOT NULL AUTO_INCREMENT,
    user_id        BIGINT    NOT NULL,
    event_id       BIGINT    NOT NULL,
    status         ENUM('HELD', 'PAID', 'EXPIRED', 'CANCELLED') NOT NULL DEFAULT 'HELD',
    total_harga    BIGINT    NOT NULL,           -- jumlah harga tahap semua tiket
    total_potongan BIGINT    NOT NULL DEFAULT 0, -- potongan promo (diisi di feat/promo)
    total_bayar    BIGINT    NOT NULL,           -- total_harga - total_potongan
    expires_at     DATETIME  NOT NULL,           -- batas bayar: waktu pesan + 15 menit
    paid_at        DATETIME  NULL,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NULL,
    PRIMARY KEY (id),
    -- worker kedaluwarsa mencari pesanan HELD yang expires_at-nya sudah lewat
    KEY idx_orders_status_expires (status, expires_at),
    CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_orders_event FOREIGN KEY (event_id) REFERENCES events (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Satu baris = satu tiket untuk satu pemegang (BR-07).
-- price_tier_id mencatat tahap asal tiket (harga yang dibayar, BR-08).
-- kode_tiket NULL selama HELD, diisi saat dibayar (BR-11). UNIQUE tetap mengizinkan banyak NULL.
CREATE TABLE IF NOT EXISTS tickets (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    order_id      BIGINT       NOT NULL,
    category_id   BIGINT       NOT NULL,
    price_tier_id BIGINT       NOT NULL,
    nama_pemegang VARCHAR(100) NOT NULL,
    harga         BIGINT       NOT NULL,           -- harga tahap
    potongan      BIGINT       NOT NULL DEFAULT 0, -- potongan promo untuk tiket ini
    harga_bayar   BIGINT       NOT NULL,           -- harga - potongan (dasar refund BR-18)
    kode_tiket    VARCHAR(16)  NULL,
    status        ENUM('HELD', 'ACTIVE', 'USED', 'CANCELLED', 'REFUNDED') NOT NULL DEFAULT 'HELD',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_tickets_kode (kode_tiket),
    CONSTRAINT fk_tickets_order FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT fk_tickets_category FOREIGN KEY (category_id) REFERENCES ticket_categories (id),
    CONSTRAINT fk_tickets_tier FOREIGN KEY (price_tier_id) REFERENCES price_tiers (id),
    CONSTRAINT chk_tickets_harga CHECK (harga >= 0 AND potongan >= 0 AND harga_bayar = harga - potongan)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
