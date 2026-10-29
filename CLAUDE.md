# CLAUDE.md — Sistem Penjualan Tiket Event

## Tentang proyek
Tugas mata kuliah Pemrograman Web: Topik 6, Sistem Penjualan Tiket Event Komunitas Musik Kampus.
Proyek ini saya kerjakan sendiri, jadi **saya harus bisa menjelaskan seluruh kode saat presentasi**.
Spesifikasi lengkap ada di `docs/PRD.md`. Baca file itu dulu sebelum merencanakan atau menulis kode.

## Gaya kode: ikuti contoh dosen
Folder materi (di luar repo ini, folder induk `Pemrograman Web/`) berisi contoh dari dosen.
Rujukan utama: **`go-class dosen/main.go`**, **`Pertemuan CRUD/`**, dan `go-class dosen/templates/`.
Baca ulang rujukan itu sebelum menulis handler, query, atau template baru, lalu ikuti polanya.

Pola dosen yang harus dipakai:
- Router: `http.NewServeMux()` dengan pola metode + path, mis. `mux.HandleFunc("GET /produk/{id}", ...)`, ambil parameter dengan `r.PathValue("id")`.
- Handler berbentuk closure: `func xHandler(db *sql.DB) http.HandlerFunc { return func(w, r) {...} }`.
- Fungsi query terpisah dan menerima `ctx`: `func getXById(ctx context.Context, db *sql.DB, id int) (X, error)`, memakai `QueryRowContext`, `QueryContext`, `ExecContext` dan placeholder `?`.
- Template: `layout.html` + `header`, `sidebar`, `footer`, dan tiap halaman mendefinisikan `{{define "content"}}`. Dimuat lewat helper `parseHalaman(...)`, dirender dengan `ExecuteTemplate(w, "layout", data)`. `template.FuncMap` untuk fungsi seperti `rupiah`.
- Form memakai POST lalu `http.Redirect(..., http.StatusSeeOther)` (pola Post/Redirect/Get).
- Hapus data dengan soft delete (kolom `deleted_at`), kolom `created_at` dan `updated_at`.
- Konfigurasi dari `.env` lewat `godotenv`, variabel `DB_DSN`.
- Tampilan: pakai CSS/layout bergaya dosen (sidebar + header + footer). Tidak perlu dibuat lebih mewah.

Yang **tidak boleh ditiru** dari contoh dosen (itu penyederhanaan untuk kelas):
- `log.Fatal(err)` di dalam handler. Itu mematikan seluruh server saat satu request error. Pakai `http.Error(...)` atau tampilkan pesan di halaman.
- Mengabaikan error dari `strconv.Atoi(...)`. Validasi input dan balas 400 jika tidak valid.
- Menyimpan `.env` tanpa `.gitignore`. Di proyek ini `.env` wajib di-ignore.

## Stack
- Go, ikuti versi Go di materi dosen. Standard library `net/http` dan `html/template`. Tanpa framework.
- MySQL 8 lewat Docker Compose (contoh: `Pertemuan CRUD/docker-compose.yml`), driver `github.com/go-sql-driver/mysql`, query manual pakai `database/sql`, tanpa ORM.
- `github.com/joho/godotenv` untuk membaca `.env` (sama seperti dosen).
- Satu-satunya library tambahan yang boleh: `golang.org/x/crypto/bcrypt` untuk hash password. Library lain **tanya dulu**.
- Migrasi berupa file `.sql` bernomor di `migrations/`, dijalankan manual seperti `database.sql` milik dosen.

## Struktur folder
```
cmd/web/main.go     start server, koneksi DB, daftar route
internal/
  event/            event, kategori tiket, tahap harga
  order/            pemesanan, hold 15 menit, pembayaran simulasi
  promo/            kode promo
  ticket/           pembatalan, refund, resale, transfer
  checkin/          pemeriksaan tiket di pintu
  report/           laporan penjualan & pendapatan
  auth/             login dan peran
templates/          layout, header, sidebar, footer, + folder per fitur
migrations/         skema & data contoh SQL
docs/PRD.md         spesifikasi
docker-compose.yml  MySQL lokal
.env.example        contoh konfigurasi (tanpa rahasia)
```
Di tiap paket: `handler.go` (HTTP, gaya closure), `query.go` (fungsi SQL), `service.go` (aturan bisnis murni Go, tanpa HTTP, supaya mudah dites), dan `*_test.go`.
Tetap sederhana: jika satu fitur kecil, boleh digabung dalam lebih sedikit file.

## Perintah
- Nyalakan DB: `docker compose up -d`
- Jalankan: `go run ./cmd/web` (default `:8080`)
- Test: `go test ./...`
- Cek: `go vet ./...` dan `gofmt -l .` (harus kosong)
- Migrasi: jalankan file di `migrations/` berurutan

## Aturan kerja (wajib)
1. **Plan dulu.** Untuk setiap fitur, tulis rencana singkat (file yang diubah, tabel, test) lalu tunggu persetujuan saya.
2. **Jangan pernah commit atau push langsung ke `main`.** Buat branch: `feat/<nama>`, `fix/<nama>`, `chore/<nama>`.
3. **Commit kecil tapi sering.** Satu commit untuk satu perubahan logis, format pesan `feat: ...`, `fix: ...`, `test: ...`, `docs: ...`, `chore: ...`.
4. Push ke branch sendiri, lalu buka Pull Request (`gh pr create`). Merge hanya jika CI hijau dan sudah saya review.
5. `.env`, binary, dan file rahasia tidak boleh di-commit. Sediakan `.env.example`. Folder materi dosen tidak boleh masuk repo.
6. Jalankan `go vet`, `gofmt`, dan `go test` sebelum setiap commit.

## Aturan kode
- Kode **sederhana dan mudah dibaca**, lebih baik panjang tapi jelas daripada pintar tapi rumit.
- Beri komentar berbahasa Indonesia di bagian logika bisnis (kuota, tahap harga, refund, dll), seperti gaya komentar dosen.
- Setiap operasi yang mengubah kuota atau status tiket **wajib dalam transaksi DB** (`db.BeginTx`) dan memakai `SELECT ... FOR UPDATE` agar tidak terjadi oversell.
- Uang disimpan sebagai integer rupiah (`BIGINT`), tidak pakai float.
- Waktu memakai zona `Asia/Jakarta` secara konsisten (`parseTime=true&loc=Asia%2FJakarta` di DSN).
- Setiap aturan bisnis bernomor (BR-xx) di PRD punya minimal satu test.
- Tangani error secara eksplisit dan tampilkan pesan yang jelas ke pengguna.

## Cara menjelaskan ke saya
Setiap kali selesai satu tugas, jelaskan dalam bahasa Indonesia sederhana:
1. File apa yang berubah dan kenapa
2. Alur datanya (dari klik pengguna sampai ke database)
3. Aturan bisnis mana (BR-xx) yang ditangani
4. 2–3 pertanyaan yang mungkin ditanyakan dosen beserta jawabannya
Jika memakai teknik di luar materi dosen, sebutkan dengan jelas dan jelaskan alasannya.
