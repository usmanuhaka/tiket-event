# tiket-event

Sistem Penjualan Tiket Event Komunitas Musik Kampus, proyek mata kuliah Pemrograman Web (Topik 6).
Ditulis dengan Go (`net/http` + `html/template`) dan MySQL/MariaDB, tanpa framework.
Spesifikasi, aturan bisnis (BR-01 s.d. BR-28), serta asumsi dan keputusan desain ada di [docs/PRD.md](docs/PRD.md).

## Fitur
- Pengelolaan event, kategori tiket, dan tahap harga (admin)
- Pemesanan 1–4 tiket dengan penahanan 15 menit dan pembayaran simulasi (pembeli)
- Kode promo per kategori
- Pembatalan tiket dengan refund, pembatalan event, dan pemindahan tiket
- Pemeriksaan tiket di pintu masuk (petugas)
- Laporan penjualan dan pendapatan per event

## Cara Menjalankan

Ada dua cara menjalankan aplikasi. Pilih **salah satu**:

| | Cara 1: XAMPP | Cara 2: Container (Podman/Docker) |
|---|---|---|
| Database | MariaDB 10.4 bawaan XAMPP | MySQL 8 di container |
| Go di komputer | Perlu (Go 1.26.6) | Tidak perlu (dibangun di dalam image) |
| Migrasi | Manual, jalankan file satu per satu | Otomatis saat volume database pertama dibuat |
| Cocok untuk | Pengembangan dan debugging lokal | Mencoba cepat dan lingkungan yang konsisten |

Daftar file migrasi (dipakai oleh kedua cara):

| File | Isi |
|---|---|
| `001_users.sql` | tabel `users` dan `sessions` |
| `002_data_contoh_users.sql` | akun demo (lihat bagian [Akun demo](#akun-demo)) |
| `003_events.sql` | tabel `events`, `ticket_categories`, `price_tiers` |
| `004_data_contoh_events.sql` | 2 event contoh dengan tanggal relatif dari saat file dijalankan: "Konser Akhir Tahun" (OPEN, ±45 hari lagi) dan "Seminar Produksi Musik" (DRAFT, sengaja belum lengkap untuk contoh "Buka Penjualan" yang ditolak) |
| `005_orders.sql` | tabel `orders` dan `tickets` |
| `006_tiers_kuota_nol.sql` | mengizinkan kuota tahap 0 (hasil pemindahan kuota, lihat PRD §9) |
| `007_promo.sql` | tabel `promo_codes`, `promo_categories`, dan kolom `orders.promo_code_id` |
| `008_refunds.sql` | tabel `refunds` (pengembalian dana per tiket) |
| `009_transfers.sql` | kolom `tickets.sudah_dipindah` dan tabel `ticket_transfers` (kode lama/baru) |
| `010_checkins.sql` | tabel `checkins` (tiket yang sudah masuk, petugas, waktu masuk) |

---

## Cara 1: Setup Database dan Menjalankan dengan XAMPP

### Kebutuhan
- Go 1.26.6
- XAMPP (MariaDB 10.4)

### 1. Siapkan database
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
   Ganti `001_nama.sql` dengan nama file sebenarnya. Bisa juga lewat menu **Import** di phpMyAdmin.
   Jalankan hanya file yang belum pernah dijalankan.

   File `*_data_contoh_*.sql` cukup dijalankan ke `tiket_event` (test membuat datanya sendiri),
   dan aman dijalankan ulang karena memakai `INSERT IGNORE`.

   Karena tanggal di `004` dihitung dengan `DATE_ADD(NOW(), ...)`, menjalankan ulang file itu tidak menggeser tanggal
   event yang sudah ada (`INSERT IGNORE` melewatinya). Untuk data baru, hapus dulu event contoh lewat phpMyAdmin.

### 2. Konfigurasi
Salin `.env.example` menjadi `.env`. Contoh tersebut memakai database container, jadi untuk XAMPP bawaan
ganti koneksinya menjadi:
```dotenv
DB_DSN=root@tcp(127.0.0.1:3306)/tiket_event?parseTime=true&loc=Asia%2FJakarta&time_zone=%27%2B07%3A00%27
TEST_DB_DSN=root@tcp(127.0.0.1:3306)/tiket_event_test?parseTime=true&loc=Asia%2FJakarta&time_zone=%27%2B07%3A00%27
```
File `.env` tidak ikut di-commit.

### 3. Jalankan aplikasi
```powershell
go run ./cmd/web
```
Buka `http://localhost:8080`. Perintah dijalankan dari root proyek karena template dibaca dari folder `templates/`.

---

## Cara 2: Setup Database dan Menjalankan dengan Container (Podman atau Docker)

`Dockerfile` membangun binary Go statis dan menjalankannya sebagai pengguna non-root.
MySQL 8 berjalan di container terpisah; port database tidak dibuka ke host.
Semua file `migrations/*.sql` dijalankan otomatis berurutan saat volume database pertama kali dibuat,
termasuk akun dan event demo. Ini adalah konfigurasi pengembangan lokal, bukan produksi.

### Kebutuhan
- Podman atau Docker
- Provider Compose (`podman compose`/`podman-compose`, atau Docker Compose) untuk opsi 2A

### Siapkan `.env`
Dari root proyek:
```sh
cp .env.example .env
chmod 600 .env
```

Ganti `MYSQL_ROOT_PASSWORD` dan `MYSQL_PASSWORD` dengan password acak, lalu samakan password
pada `DB_DSN` dengan `MYSQL_PASSWORD`. Gunakan password heksadesimal agar tidak perlu escaping
pada DSN/Compose (contoh generator: `openssl rand -hex 24`).
`DB_DSN` memakai host `db:3306`, user `MYSQL_USER`, dan database `MYSQL_DATABASE`.

### Opsi 2A: Compose (disarankan)

```sh
podman compose up -d --build     # Docker: docker compose up -d --build
podman compose logs -f           # Docker: docker compose logs -f
```

Buka **http://localhost:8080**. Aplikasi hanya dipublikasikan ke loopback host.
Container web menunggu MySQL sehat sebelum mulai.

```sh
podman compose down       # hentikan container; data tetap tersimpan
podman compose down -v    # PERHATIAN: hapus juga seluruh data database
```

Catatan penting:
- Migrasi baru tidak otomatis dijalankan pada volume yang sudah berisi database.
  Jalankan hanya file migrasi baru secara manual; jangan menghapus volume untuk upgrade data penting.
- Mengubah password di `.env` juga tidak mengubah password pada database yang sudah diinisialisasi.

### Opsi 2B: Tanpa provider Compose

Setelah menyiapkan `.env` seperti di atas, perintah berikut menjalankan konfigurasi yang sama
menggunakan Podman langsung. Jalankan dari root proyek. Ekspor variabel tanpa mencetak password:

```sh
set -a
. ./.env
set +a
podman build -t localhost/tiket-event:latest .
podman network create tiket-event
podman volume create tiket-event-mysql
podman run -d --name tiket-event-db --network tiket-event --network-alias db \
  --restart unless-stopped \
  --env MYSQL_ROOT_PASSWORD --env MYSQL_DATABASE --env MYSQL_USER --env MYSQL_PASSWORD \
  -v tiket-event-mysql:/var/lib/mysql \
  -v "$(pwd)/migrations:/docker-entrypoint-initdb.d:ro,Z" \
  --health-cmd='MYSQL_PWD="$MYSQL_PASSWORD" mysql --protocol=TCP -h 127.0.0.1 -u"$MYSQL_USER" "$MYSQL_DATABASE" -e "SELECT 1" >/dev/null 2>&1' \
  --health-interval=5s --health-timeout=5s --health-retries=30 --health-start-period=30s \
  docker.io/library/mysql:8.0 \
  --character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci
```

Tunggu sampai `podman inspect --format '{{.State.Health.Status}}' tiket-event-db`
menampilkan `healthy`, lalu mulai web:

```sh
podman run -d --name tiket-event-web --network tiket-event \
  --restart unless-stopped --env DB_DSN \
  -p 127.0.0.1:8080:8080 localhost/tiket-event:latest
podman logs tiket-event-web
```

Untuk berhenti: `podman stop tiket-event-web tiket-event-db`.
Untuk mulai lagi: `podman start tiket-event-db`, tunggu sehat, lalu `podman start tiket-event-web`.

Catatan:
- Jangan menjalankan cara langsung dan Compose bersamaan karena nama container/volume sama.
- Pengguna Docker bisa memakai perintah yang sama dengan mengganti `podman` menjadi `docker`
  (opsi `:Z` pada volume hanya berlaku di sistem dengan SELinux; hapus jika Docker menolaknya).

---

## Akun demo
Dibuat oleh `migrations/002_data_contoh_users.sql`.

| Peran | Email | Password |
|---|---|---|
| Admin | `admin@kampus.test` | `demo12345` |
| Petugas pintu | `petugas@kampus.test` | `demo12345` |
| Pembeli | `pembeli@kampus.test` | `demo12345` |

> Akun ini hanya untuk pengembangan lokal; ganti atau hapus sebelum aplikasi dipasang di server.

Pembeli baru bisa mendaftar sendiri lewat halaman `/daftar`. Akun admin dan petugas hanya dibuat lewat data contoh.

## Test dan pengecekan
Membutuhkan Go dan database test (`tiket_event_test`, lihat Cara 1).
```powershell
go test ./...
go vet ./...
gofmt -l .
```
Test yang butuh database memakai `TEST_DB_DSN`. Jika variabel itu kosong, test tersebut di-skip.
GitHub Actions menjalankan semua pengecekan ini dengan MySQL 8 setiap ada push dan Pull Request
(`go test -p 1 -race ./...`).

## Panduan pemakaian

### Mengelola event (admin)
Menu **Kelola Event**:
1. **Tambah Event**: event baru berstatus `DRAFT`.
2. Di halaman detail, tambah **kategori** (total kuota ≤ kapasitas) lalu **tahap harga** per kategori.
   Selama `DRAFT`, kategori dan tahap bisa dihapus lalu ditambah ulang.
3. Klik **Buka Penjualan**. Jika ada syarat yang belum terpenuhi, semua masalahnya ditampilkan.
   Setelah `OPEN`, kategori dan tahap terkunci.

### Memesan tiket (pembeli)
1. Buka menu **Event**, pilih event, lalu isi 1–4 baris tiket (kategori + nama pemegang) dan klik **Pesan**.
   Harga per tiket mengikuti tahap aktif; jika sisa tahap kurang, tiket berikutnya memakai harga tahap sesudahnya.
2. Tiket ditahan **15 menit** (status `HELD`) dan kuota langsung berkurang.
3. Di halaman pesanan, klik **Konfirmasi Pembayaran** (simulasi). Pesanan menjadi `PAID` dan setiap tiket mendapat kode,
   mis. `TKT-7K3M9QX2`.
4. Jika tidak dibayar dalam 15 menit, worker (berjalan tiap 1 menit di dalam server) membuat pesanan `EXPIRED`
   dan mengembalikan kuotanya. Aturan pengembalian kuota ke tahap yang sudah lewat ada di PRD §9.

### Kode promo
- Admin membuat promo di menu **Kelola Promo**: kode (otomatis huruf besar), potongan 1–100%, kuota pemakaian,
  dan kategori tempat promo berlaku (boleh dari beberapa event).
- Pembeli mengisi **Kode Promo** di form pesan. Potongan dihitung per tiket, dibulatkan ke bawah,
  hanya untuk tiket dari kategori yang terdaftar. Kode ditolak jika tidak ada, kuotanya habis,
  atau tidak berlaku untuk satu pun tiket yang dipesan.
- Satu pesanan = satu pemakaian, dihitung saat pesanan dibuat dan dikembalikan jika pesanan `EXPIRED`
  (termasuk jika promonya sudah dihapus admin).

### Pembatalan dan refund
- Pembeli membuka pesanan yang sudah dibayar lalu klik **Batalkan** pada tiket yang ingin dibatalkan.
  Boleh paling lambat **H-7** (dihitung per tanggal kalender WIB; event 20 Des → paling lambat 13 Des).
  Refund **75%** dari harga yang dibayar (setelah promo, dibulatkan ke bawah), tiket menjadi `REFUNDED`,
  dan kuotanya kembali sehingga bisa dijual lagi.
- Admin membuka detail event `OPEN` yang belum dimulai lalu klik **Batalkan Event**.
  Semua tiket `ACTIVE` di-refund **100%**, pesanan yang belum dibayar dibatalkan, dan penjualan ditutup.
- Pemakaian promo tidak dikembalikan pada kedua jenis pembatalan.

### Pemindahan tiket
- Di detail pesanan, klik **Pindahkan** pada tiket, isi nama pemegang baru, lalu simpan.
- Hanya pemilik pesanan yang bisa memindahkan, hanya **satu kali** per tiket, untuk tiket `ACTIVE`
  yang belum dipakai masuk, selama event masih `OPEN` dan belum dimulai.
- Tiket mendapat **kode baru**; kode lama langsung tidak berlaku tetapi tetap dicatat di `ticket_transfers`.
- Tiket yang sudah dipindahkan tidak bisa dibatalkan pembeli (tetap di-refund 100% jika event dibatalkan).

### Pemeriksaan tiket di pintu (petugas)
1. Login sebagai petugas, buka menu **Pemeriksaan Tiket**, pilih event yang berlangsung **hari ini**.
2. Ketik kode tiket lalu **Periksa**: tampil nama pemegang, kategori, dan status. Jika boleh, klik **Izinkan Masuk**.
3. Tiket menjadi `USED` dan waktu masuknya dicatat. Pemindaian berikutnya ditolak "sudah dipakai masuk pada ...".

Kode lama dari tiket yang sudah dipindahkan ditolak dengan nama penerimanya. Tiket untuk event lain, bukan hari event
(tanggal WIB), belum dibayar, atau sudah dibatalkan juga ditolak dengan alasannya.

Data contoh tidak punya event hari ini. Untuk mencoba alur ini, buat event dengan tanggal hari ini dan jam yang
belum lewat (sebagai admin), tambahkan kategori dan tahap, buka penjualan, pesan dan bayar (sebagai pembeli),
lalu periksa tiketnya sebagai petugas.

### Laporan penjualan (admin)
Menu **Laporan** menampilkan semua event non-`DRAFT` beserta jumlah terjual dan pendapatan bersihnya.
**Lihat Laporan** menampilkan angka per tahap, subtotal per kategori, dan total event:

| Kolom | Arti |
|---|---|
| Terjual | tiket yang pernah dibayar: `ACTIVE`, `USED`, `REFUNDED` (`HELD`/`CANCELLED` tidak dihitung) |
| Kotor | jumlah harga tahap sebelum promo |
| Potongan Promo | jumlah potongan promo |
| Refund | jumlah uang yang dikembalikan (75% pembatalan pembeli, 100% event dibatalkan) |
| Bersih | kotor − potongan − refund (BR-28) |

Semua angka satu laporan dibaca dari satu snapshot database. Event yang dibatalkan penyelenggara bersih Rp0,
kecuali ada pembeli yang sudah membatalkan sebelumnya (sisa 25% tiket itu tetap tercatat, lihat PRD §9).