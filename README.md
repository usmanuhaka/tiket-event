# tiket-event

Sistem Penjualan Tiket Event Komunitas Musik Kampus, tugas mata kuliah Pemrograman Web (Topik 6).
Ditulis dengan Go (`net/http` + `html/template`) dan MySQL/MariaDB, tanpa framework.
Spesifikasi lengkap ada di [docs/PRD.md](docs/PRD.md).

## Kebutuhan
- Go 1.26.6
- XAMPP (MariaDB 10.4) untuk database lokal

## Menyiapkan database (XAMPP)
1. Buka **XAMPP Control Panel**, klik **Start** pada baris MySQL.
2. Buat dua database (lewat phpMyAdmin di `http://localhost/phpmyadmin`, atau perintah di bawah):
   ```powershell
   C:\xampp\mysql\bin\mysql.exe -uroot -e "CREATE DATABASE tiket_event CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; CREATE DATABASE tiket_event_test CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
   ```
   `tiket_event` dipakai aplikasi, `tiket_event_test` dipakai test integrasi.
3. Jalankan file di folder `migrations/` **berurutan sesuai nomor** ke kedua database, dari root proyek:
   ```powershell
   C:\xampp\mysql\bin\mysql.exe -uroot --default-character-set=utf8mb4 tiket_event -e "source migrations/001_nama.sql"
   C:\xampp\mysql\bin\mysql.exe -uroot --default-character-set=utf8mb4 tiket_event_test -e "source migrations/001_nama.sql"
   ```
   Bisa juga lewat menu **Import** di phpMyAdmin. Jalankan hanya file yang belum pernah dijalankan.
   File `*_data_contoh_*.sql` cukup dijalankan ke `tiket_event` (test membuat datanya sendiri),
   dan aman dijalankan ulang karena memakai `INSERT IGNORE`.

| File | Isi |
|---|---|
| `001_users.sql` | tabel `users` dan `sessions` |
| `002_data_contoh_users.sql` | akun demo (lihat di bawah) |
| `003_events.sql` | tabel `events`, `ticket_categories`, `price_tiers` |
| `004_data_contoh_events.sql` | 2 event contoh dengan tanggal relatif dari saat file dijalankan: "Konser Akhir Tahun" (OPEN, ±45 hari lagi) dan "Seminar Produksi Musik" (DRAFT, sengaja belum lengkap untuk demo "Buka Penjualan" yang ditolak) |
| `005_orders.sql` | tabel `orders` dan `tickets` |
| `006_tiers_kuota_nol.sql` | mengizinkan kuota tahap 0 (hasil pemindahan kuota, lihat PRD §9) |

Karena tanggal di `004` dihitung dengan `DATE_ADD(NOW(), ...)`, menjalankan ulang file itu tidak menggeser tanggal
event yang sudah ada (`INSERT IGNORE` melewatinya). Untuk data baru, hapus dulu event contoh lewat phpMyAdmin.

## Mengelola event (Admin)
Login sebagai admin, lalu buka menu **Kelola Event**:
1. **Tambah Event**: event baru berstatus `DRAFT`.
2. Di halaman detail, tambah **kategori** (total kuota ≤ kapasitas) lalu **tahap harga** per kategori.
   Selama DRAFT, kategori dan tahap bisa dihapus lalu ditambah ulang.
3. Klik **Buka Penjualan**. Jika ada syarat yang belum terpenuhi, semua masalahnya ditampilkan.
   Setelah `OPEN`, kategori dan tahap terkunci.

## Memesan tiket (Pembeli)
1. Buka menu **Event**, pilih event, lalu isi 1–4 baris tiket (kategori + nama pemegang) dan klik **Pesan**.
   Harga per tiket mengikuti tahap aktif; jika sisa tahap kurang, tiket berikutnya memakai harga tahap sesudahnya.
2. Tiket ditahan **15 menit** (status `HELD`) dan kuota langsung berkurang.
3. Di halaman pesanan, klik **Konfirmasi Pembayaran** (simulasi). Pesanan menjadi `PAID` dan setiap tiket mendapat kode,
   mis. `TKT-7K3M9QX2`.
4. Jika tidak dibayar dalam 15 menit, worker (berjalan tiap 1 menit di dalam server) membuat pesanan `EXPIRED`
   dan mengembalikan kuotanya. Aturan pengembalian kuota ke tahap yang sudah lewat ada di `docs/PRD.md` §9.

## Akun demo
Dibuat oleh `migrations/002_data_contoh_users.sql`.

| Peran | Email | Password |
|---|---|---|
| Admin | `admin@kampus.test` | `demo12345` |
| Petugas pintu | `petugas@kampus.test` | `demo12345` |
| Pembeli | `pembeli@kampus.test` | `demo12345` |

> **Password ini hanya untuk demo lokal.** Semua orang yang membaca repo ini bisa melihatnya,
> jadi jangan pernah menjalankan data contoh ini di server yang bisa diakses orang lain.

Pembeli baru bisa mendaftar sendiri lewat halaman `/daftar`. Akun admin dan petugas hanya dibuat lewat data contoh.

## Konfigurasi
Salin `.env.example` menjadi `.env`. Isinya sudah cocok untuk XAMPP bawaan (user `root` tanpa password).
File `.env` tidak ikut di-commit.

## Menjalankan
```powershell
go run ./cmd/web
```
Buka `http://localhost:8080`. Perintah dijalankan dari root proyek karena template dibaca dari folder `templates/`.

## Test dan pengecekan
```powershell
go test ./...
go vet ./...
gofmt -l .
```
Test yang butuh database memakai `TEST_DB_DSN`. Jika variabel itu kosong, test tersebut di-skip.
GitHub Actions menjalankan semua pengecekan ini dengan MySQL 8 setiap ada push dan Pull Request.
