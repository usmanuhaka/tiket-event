-- Akun demo untuk dicoba di lokal. Password ketiga akun: demo12345
-- (hanya untuk demo lokal, jangan dipakai di server sungguhan).
-- password_hash adalah hasil bcrypt dari password di atas.
-- INSERT IGNORE: jika email sudah ada (file dijalankan ulang), baris itu dilewati, bukan error.

INSERT IGNORE INTO users (nama, email, password_hash, role) VALUES
    ('Admin Komunitas', 'admin@kampus.test',   '$2a$10$md4jsPj7/KwFgkQV5yoqBODbgB7/ynZLOPzyPNHP5Wa4U1GJC4Fu2', 'ADMIN'),
    ('Petugas Pintu',   'petugas@kampus.test', '$2a$10$md4jsPj7/KwFgkQV5yoqBODbgB7/ynZLOPzyPNHP5Wa4U1GJC4Fu2', 'PETUGAS'),
    ('Pembeli Contoh',  'pembeli@kampus.test', '$2a$10$md4jsPj7/KwFgkQV5yoqBODbgB7/ynZLOPzyPNHP5Wa4U1GJC4Fu2', 'PEMBELI');
