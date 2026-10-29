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
