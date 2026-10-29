-- Pemindahan tiket (BR-21 s.d. BR-23).
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

-- BR-21: tiket hanya bisa dipindahkan satu kali
ALTER TABLE tickets ADD COLUMN sudah_dipindah BOOLEAN NOT NULL DEFAULT 0 AFTER status;

-- Satu baris = satu pemindahan.
-- ticket_id UNIQUE: pengaman terakhir BR-21 (tidak mungkin ada dua pemindahan untuk satu tiket).
-- kode_lama disimpan (BR-22) supaya pemeriksaan di pintu bisa memberi tahu bahwa kode itu
-- sudah tidak berlaku karena tiketnya dipindahkan. Pencarian di pintu mendahulukan
-- tickets.kode_tiket (kode yang berlaku), baru ticket_transfers.kode_lama.
-- user_id adalah pemilik pesanan yang memindahkan (hanya pemilik yang boleh, PRD §9).
CREATE TABLE IF NOT EXISTS ticket_transfers (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    ticket_id  BIGINT       NOT NULL,
    user_id    BIGINT       NOT NULL,
    nama_lama  VARCHAR(100) NOT NULL,
    nama_baru  VARCHAR(100) NOT NULL,
    kode_lama  VARCHAR(16)  NOT NULL,
    kode_baru  VARCHAR(16)  NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_transfers_ticket (ticket_id),
    UNIQUE KEY uk_transfers_kode_lama (kode_lama),
    CONSTRAINT fk_transfers_ticket FOREIGN KEY (ticket_id) REFERENCES tickets (id),
    CONSTRAINT fk_transfers_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
