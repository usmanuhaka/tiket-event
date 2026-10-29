-- Data contoh event untuk demo lokal. Cukup dijalankan ke tiket_event.
-- Tanggal dihitung dari saat file dijalankan (DATE_ADD(NOW(), ...)), jadi event selalu di masa depan.
-- id ditulis tetap supaya INSERT IGNORE melewati baris yang sudah ada jika file dijalankan ulang.

-- NOW() memakai zona waktu sesi; samakan dengan aplikasi (Asia/Jakarta, +07:00)
SET time_zone = '+07:00';

-- Event 1: sudah OPEN, siap dipesan.
-- Event 2: masih DRAFT dan sengaja belum lengkap (tahap baru 200 dari kuota 300),
--          untuk mendemokan "Buka Penjualan" yang ditolak karena BR-04.
INSERT IGNORE INTO events (id, nama, lokasi, deskripsi, tanggal_event, kapasitas, status) VALUES
    (1, 'Konser Akhir Tahun', 'Auditorium Kampus', 'Konser penutup tahun Komunitas Musik Kampus.',
        DATE_ADD(NOW(), INTERVAL 45 DAY), 500, 'OPEN'),
    (2, 'Seminar Produksi Musik', 'Gedung Serbaguna', 'Seminar rekaman dan mixing untuk pemula.',
        DATE_ADD(NOW(), INTERVAL 60 DAY), 300, 'DRAFT');

-- Event 1: Festival 400 + VIP 100 = 500 = kapasitas (BR-02)
INSERT IGNORE INTO ticket_categories (id, event_id, nama, kuota) VALUES
    (1, 1, 'Festival', 400),
    (2, 1, 'VIP', 100),
    (3, 2, 'Umum', 300);

-- Tiap kategori event 1 punya 3 tahap; total kuota tahap = kuota kategori (BR-04)
-- Festival: 100 + 150 + 150 = 400, VIP: 30 + 30 + 40 = 100
INSERT IGNORE INTO price_tiers (id, category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal) VALUES
    (1, 1, 'Presale 1', 1, 150000, 100, 100, DATE_ADD(NOW(), INTERVAL 7 DAY)),
    (2, 1, 'Presale 2', 2, 175000, 150, 150, DATE_ADD(NOW(), INTERVAL 14 DAY)),
    (3, 1, 'Normal',    3, 200000, 150, 150, DATE_ADD(NOW(), INTERVAL 30 DAY)),
    (4, 2, 'Presale 1', 1, 350000,  30,  30, DATE_ADD(NOW(), INTERVAL 7 DAY)),
    (5, 2, 'Presale 2', 2, 400000,  30,  30, DATE_ADD(NOW(), INTERVAL 14 DAY)),
    (6, 2, 'Normal',    3, 450000,  40,  40, DATE_ADD(NOW(), INTERVAL 30 DAY)),
    (7, 3, 'Normal',    1,  50000, 200, 200, DATE_ADD(NOW(), INTERVAL 30 DAY));
