-- Pengembalian dana (BR-17 s.d. BR-20).
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

-- Satu baris = satu refund untuk satu tiket.
--   persen 75  : pembeli membatalkan tiket ACTIVE paling lambat H-7 (BR-17, BR-18)
--   persen 100 : event dibatalkan penyelenggara (BR-20)
-- user_id adalah penerima refund, yaitu pembeli pemilik pesanan (keputusan di PRD §9).
-- ticket_id UNIQUE: satu tiket tidak mungkin di-refund dua kali.
CREATE TABLE IF NOT EXISTS refunds (
    id         BIGINT    NOT NULL AUTO_INCREMENT,
    ticket_id  BIGINT    NOT NULL,
    order_id   BIGINT    NOT NULL,
    user_id    BIGINT    NOT NULL,
    jumlah     BIGINT    NOT NULL,
    persen     INT       NOT NULL,
    alasan     ENUM('PEMBELI', 'EVENT_DIBATALKAN') NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_refunds_ticket (ticket_id),
    CONSTRAINT fk_refunds_ticket FOREIGN KEY (ticket_id) REFERENCES tickets (id),
    CONSTRAINT fk_refunds_order FOREIGN KEY (order_id) REFERENCES orders (id),
    CONSTRAINT fk_refunds_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT chk_refunds_jumlah CHECK (jumlah >= 0),
    CONSTRAINT chk_refunds_persen CHECK (persen IN (75, 100))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
