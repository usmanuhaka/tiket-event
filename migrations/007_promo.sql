-- Kode promo (BR-13 s.d. BR-16).
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

-- BR-13: kode unik, persentase potongan 1-100, kuota pemakaian.
-- kode selalu disimpan huruf besar (HEMAT15), jadi "hemat15" dan "HEMAT15" dianggap sama.
-- terpakai: berapa pesanan yang sedang/sudah memakai promo ini (BR-15).
CREATE TABLE IF NOT EXISTS promo_codes (
    id              BIGINT      NOT NULL AUTO_INCREMENT,
    kode            VARCHAR(30) NOT NULL,
    persen          INT         NOT NULL,
    kuota_pemakaian INT         NOT NULL,
    terpakai        INT         NOT NULL DEFAULT 0,
    created_at      TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP   NULL,
    deleted_at      TIMESTAMP   NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_promo_kode (kode),
    CONSTRAINT chk_promo_persen CHECK (persen BETWEEN 1 AND 100),
    CONSTRAINT chk_promo_kuota CHECK (kuota_pemakaian > 0),
    -- BR-16: pengaman terakhir, pemakaian tidak boleh melebihi kuota
    CONSTRAINT chk_promo_terpakai CHECK (terpakai >= 0 AND terpakai <= kuota_pemakaian)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- BR-13/BR-14: daftar kategori tempat promo berlaku (boleh dari beberapa event)
CREATE TABLE IF NOT EXISTS promo_categories (
    promo_code_id BIGINT NOT NULL,
    category_id   BIGINT NOT NULL,
    PRIMARY KEY (promo_code_id, category_id),
    CONSTRAINT fk_promo_categories_promo FOREIGN KEY (promo_code_id) REFERENCES promo_codes (id),
    CONSTRAINT fk_promo_categories_category FOREIGN KEY (category_id) REFERENCES ticket_categories (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- pesanan mencatat promo yang dipakai (NULL jika tanpa promo)
ALTER TABLE orders ADD COLUMN promo_code_id BIGINT NULL AFTER event_id;
ALTER TABLE orders ADD CONSTRAINT fk_orders_promo FOREIGN KEY (promo_code_id) REFERENCES promo_codes (id);
