-- Tabel event, kategori tiket, dan tahap harga (BR-01 s.d. BR-05).
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

-- Status event:
--   DRAFT     admin masih menyusun kategori dan tahap, belum bisa dipesan
--   OPEN      penjualan dibuka, struktur kuota dikunci
--   CANCELLED event dibatalkan penyelenggara (BR-20)
CREATE TABLE IF NOT EXISTS events (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    nama          VARCHAR(150) NOT NULL,
    lokasi        VARCHAR(150) NOT NULL,
    deskripsi     TEXT         NULL,
    tanggal_event DATETIME     NOT NULL,
    kapasitas     INT          NOT NULL,
    status        ENUM('DRAFT', 'OPEN', 'CANCELLED') NOT NULL DEFAULT 'DRAFT',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NULL,
    deleted_at    TIMESTAMP    NULL,
    PRIMARY KEY (id),
    CONSTRAINT chk_events_kapasitas CHECK (kapasitas > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- BR-01: satu event punya beberapa kategori, tiap kategori punya kuota.
-- BR-02 (total kuota kategori <= kapasitas event) dicek di kode Go dalam transaksi.
CREATE TABLE IF NOT EXISTS ticket_categories (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    event_id   BIGINT       NOT NULL,
    nama       VARCHAR(100) NOT NULL,
    kuota      INT          NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP    NULL,
    deleted_at TIMESTAMP    NULL,
    PRIMARY KEY (id),
    CONSTRAINT fk_categories_event FOREIGN KEY (event_id) REFERENCES events (id),
    CONSTRAINT chk_categories_kuota CHECK (kuota > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- BR-03: satu kategori punya beberapa tahap harga berurutan (Presale 1, Presale 2, Normal).
-- urutan diisi otomatis (urutan terakhir + 1). sisa_kuota berkurang saat tiket dipesan.
-- BR-04 (total kuota tahap = kuota kategori) dicek di kode Go saat penjualan dibuka.
CREATE TABLE IF NOT EXISTS price_tiers (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    category_id   BIGINT       NOT NULL,
    nama          VARCHAR(100) NOT NULL,
    urutan        INT          NOT NULL,
    harga         BIGINT       NOT NULL,
    kuota         INT          NOT NULL,
    sisa_kuota    INT          NOT NULL,
    batas_tanggal DATETIME     NOT NULL,
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NULL,
    deleted_at    TIMESTAMP    NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_tiers_urutan (category_id, urutan),
    CONSTRAINT fk_tiers_category FOREIGN KEY (category_id) REFERENCES ticket_categories (id),
    CONSTRAINT chk_tiers_harga CHECK (harga >= 0),
    CONSTRAINT chk_tiers_kuota CHECK (kuota > 0),
    -- pengaman terakhir anti-oversell: sisa kuota tidak boleh minus atau melebihi kuota
    CONSTRAINT chk_tiers_sisa CHECK (sisa_kuota >= 0 AND sisa_kuota <= kuota)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
