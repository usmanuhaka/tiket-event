-- Tabel pengguna dan sesi login.
-- Jalankan ke database tiket_event dan tiket_event_test (lihat README).

CREATE TABLE IF NOT EXISTS users (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    nama          VARCHAR(100) NOT NULL,
    email         VARCHAR(150) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          ENUM('ADMIN', 'PEMBELI', 'PETUGAS') NOT NULL DEFAULT 'PEMBELI',
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NULL,
    deleted_at    TIMESTAMP    NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Satu baris = satu login yang masih aktif.
-- token disimpan juga di cookie browser; logout menghapus barisnya.
CREATE TABLE IF NOT EXISTS sessions (
    id         BIGINT    NOT NULL AUTO_INCREMENT,
    token      CHAR(64)  NOT NULL,
    user_id    BIGINT    NOT NULL,
    expires_at DATETIME  NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_sessions_token (token),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
