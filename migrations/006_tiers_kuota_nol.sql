-- Izinkan kuota tahap bernilai 0.
-- Alasan: saat kuota dikembalikan ke tahap yang batas tanggalnya sudah lewat, 1 kuota dipindahkan
-- ke tahap berikutnya (keputusan BR-10/BR-19 di PRD §9). Kuota tahap asal berkurang 1,
-- sehingga tahap yang semula berkuota 1 bisa menjadi 0. Tahap berkuota 0 disembunyikan di tampilan.
-- Form admin tetap mewajibkan kuota > 0 saat menambah tahap (dicek di kode Go).
-- DROP CONSTRAINT dan ADD CONSTRAINT dikenali MariaDB 10.4 dan MySQL 8.

ALTER TABLE price_tiers DROP CONSTRAINT chk_tiers_kuota;
ALTER TABLE price_tiers ADD CONSTRAINT chk_tiers_kuota CHECK (kuota >= 0);
