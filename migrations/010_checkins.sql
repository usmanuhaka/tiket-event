-- Pemeriksaan tiket di pintu masuk (BR-24 s.d. BR-26).
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

-- Satu baris = satu tiket yang sudah masuk.
-- ticket_id UNIQUE (BR-26): satu tiket hanya bisa dipakai masuk satu kali.
-- waktu_masuk dicatat untuk pesan "sudah dipakai masuk pada ..." dan laporan.
CREATE TABLE IF NOT EXISTS checkins (
    id          BIGINT    NOT NULL AUTO_INCREMENT,
    ticket_id   BIGINT    NOT NULL,
    event_id    BIGINT    NOT NULL,
    petugas_id  BIGINT    NOT NULL,
    waktu_masuk DATETIME  NOT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_checkins_ticket (ticket_id),
    CONSTRAINT fk_checkins_ticket FOREIGN KEY (ticket_id) REFERENCES tickets (id),
    CONSTRAINT fk_checkins_event FOREIGN KEY (event_id) REFERENCES events (id),
    CONSTRAINT fk_checkins_petugas FOREIGN KEY (petugas_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
