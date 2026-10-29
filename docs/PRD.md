# PRD — Sistem Penjualan Tiket Event Komunitas Musik Kampus

## 1. Latar belakang
Komunitas Musik Kampus rutin mengadakan konser dan seminar dengan kapasitas 300–1.000 orang.
Selama ini penjualan memakai formulir daring dan konfirmasi transfer manual, sehingga pernah terjadi
**tiket terjual melebihi kapasitas**. Sistem ini menggantikan proses tersebut.

## 2. Pengguna (peran)
| Peran | Bisa melakukan |
|---|---|
| Penyelenggara (admin) | Kelola event, kategori, tahap harga, kode promo; membatalkan event; melihat laporan |
| Pembeli | Memesan tiket, membayar (simulasi), membatalkan, memindahkan tiket |
| Petugas pintu | Memeriksa tiket dengan kode tiket |

Login cukup sederhana (email + password, password di-hash dengan bcrypt), dan peran disimpan di tabel user.

## 3. Lingkup fitur
1. Pengelolaan event, kategori tiket, dan tahapan harga
2. Pemesanan tiket dengan penahanan sementara dan konfirmasi pembayaran
3. Pengelolaan kode promo
4. Pembatalan tiket, pengembalian dana, dan pemindahan tiket
5. Pemeriksaan tiket di pintu masuk
6. Laporan penjualan dan pendapatan

## 4. Aturan bisnis

### Event, kategori, tahap
- **BR-01** Satu event memiliki beberapa kategori tiket (mis. Festival, VIP), dan tiap kategori punya kuota.
- **BR-02** Total kuota semua kategori tidak boleh melebihi kapasitas event.
- **BR-03** Satu kategori memiliki beberapa tahap harga berurutan (mis. Presale 1, Presale 2, Normal),
  dan tiap tahap punya harga, kuota, dan batas tanggal.
- **BR-04** Jumlah kuota semua tahap dalam satu kategori sama dengan kuota kategori.
- **BR-05** Tahap aktif adalah tahap pertama (berdasarkan urutan) yang **masih punya sisa kuota DAN batas tanggalnya belum lewat**.
  Jika tidak ada tahap aktif, kategori tersebut habis.

### Pemesanan
- **BR-06** Satu transaksi berisi 1–4 tiket dari satu event.
- **BR-07** Setiap tiket wajib mencantumkan nama pemegang.
- **BR-08** Harga ditentukan per tiket dengan alokasi berurutan. Jika tahap aktif hanya tersisa 1 kuota dan pembeli memesan 3,
  maka 1 tiket memakai harga tahap itu dan 2 tiket memakai harga tahap berikutnya.
- **BR-09** Saat dipesan, tiket langsung **mengurangi kuota** dan berstatus `HELD` selama **15 menit**.
- **BR-10** Jika belum dibayar dalam 15 menit, pesanan menjadi `EXPIRED` dan kuota dikembalikan ke tahap asal.
  Pengecekan dilakukan oleh worker berkala (tiap 1 menit) dan juga dicek ulang saat pembayaran.
- **BR-11** Pembayaran disimulasikan dengan tombol "Konfirmasi Pembayaran". Jika masih dalam batas waktu,
  pesanan menjadi `PAID`, tiket menjadi `ACTIVE`, dan setiap tiket mendapat kode unik.
- **BR-12** Tidak boleh terjadi oversell walaupun banyak orang memesan bersamaan. Semua perubahan kuota
  dilakukan dalam transaksi DB dengan row lock.

### Kode promo
- **BR-13** Promo memiliki kode unik, persentase potongan (1–100), kuota pemakaian, dan daftar kategori yang berlaku.
- **BR-14** Potongan hanya berlaku untuk tiket dari kategori yang terdaftar dalam promo.
- **BR-15** Satu pemakaian = satu transaksi. Pemakaian dihitung saat pesanan dibuat, lalu dikembalikan jika pesanan `EXPIRED`.
- **BR-16** Promo yang kuotanya habis tidak bisa dipakai.

### Pembatalan dan pengembalian dana
- **BR-17** Pembeli dapat membatalkan tiket `ACTIVE` paling lambat **7 hari sebelum tanggal event**.
- **BR-18** Refund untuk pembatalan oleh pembeli adalah **75% dari harga yang dibayar** untuk tiket itu (setelah potongan promo).
- **BR-19** Tiket yang dibatalkan pembeli mengembalikan kuota ke tahap asalnya, sehingga **bisa dijual kembali**.
- **BR-20** Jika event dibatalkan penyelenggara, semua tiket `ACTIVE` dibatalkan dengan **refund 100%**,
  pesanan `HELD` dibatalkan, dan penjualan ditutup.

### Pemindahan tiket
- **BR-21** Pemegang tiket dapat memindahkan tiket ke orang lain **satu kali** dengan mengganti nama pemegang.
- **BR-22** Saat dipindahkan, **kode tiket diganti baru** sehingga kode lama tidak berlaku lagi.
- **BR-23** Hanya tiket `ACTIVE` yang belum dipakai masuk dan event-nya belum lewat yang bisa dipindahkan.

### Pemeriksaan di pintu
- **BR-24** Petugas memasukkan kode tiket. Sistem menampilkan nama pemegang, kategori, dan status.
- **BR-25** Tiket hanya diterima jika `ACTIVE`, untuk event yang sesuai, dan pada hari event.
- **BR-26** Tiket hanya bisa dipakai masuk **satu kali**. Setelah check-in statusnya menjadi `USED` dan waktu masuknya dicatat.

### Laporan
- **BR-27** Laporan per event menampilkan jumlah tiket terjual dan pendapatan kotor **per kategori dan per tahap**.
- **BR-28** Pendapatan bersih = pendapatan kotor − total potongan promo − total refund.

## 5. Status
- Pesanan: `HELD` → `PAID` | `EXPIRED` | `CANCELLED`
- Tiket: `HELD` → `ACTIVE` → `USED` | `CANCELLED` | `REFUNDED`

## 6. Gambaran tabel
`users`, `events`, `ticket_categories`, `price_tiers`, `orders`, `tickets`,
`promo_codes`, `promo_categories`, `refunds`, `ticket_transfers`, `checkins`

## 7. Di luar lingkup
- Payment gateway sungguhan (cukup tombol simulasi)
- Pengiriman email/WhatsApp
- QR code (opsional, cukup kode teks)
- Tampilan mewah (HTML sederhana sudah cukup)

## 8. Kriteria selesai
- Semua BR punya test otomatis dan `go test ./...` hijau
- Ada test pemesanan bersamaan yang membuktikan tidak ada oversell (BR-12)
- Alur lengkap bisa didemokan: buat event → pesan → bayar → transfer → check-in → lihat laporan
- Saya bisa menjelaskan setiap aturan dan di mana kodenya berada

## 9. Asumsi yang saya ambil (soal tidak menyebutkan)
BR-04, BR-08, BR-10 (kuota kembali ke tahap asal), BR-15, BR-18 (dihitung dari harga setelah promo), BR-22, BR-23.
Siapkan alasan untuk tiap asumsi ini saat presentasi, atau sesuaikan jika dosen punya ketentuan lain.

### Keputusan: kuota kembali ke tahap yang batas tanggalnya sudah lewat (BR-10, BR-19)
**Masalah.** Kuota dari pesanan yang `EXPIRED` (BR-10) atau tiket yang dibatalkan pembeli (BR-19)
dikembalikan "ke tahap asal". Jika batas tanggal tahap asal sudah lewat, tahap itu tidak akan aktif lagi
(BR-05), sehingga kursinya tidak bisa dijual ke siapa pun.

**Keputusan (Opsi B).** Saat 1 kuota dikembalikan:
1. Jika tahap asal masih berlaku (batas tanggal belum lewat), kuota kembali ke tahap asal: `sisa_kuota + 1`.
2. Jika tahap asal sudah lewat, 1 kuota **dipindahkan** ke tahap berikutnya (urutan lebih besar)
   yang masih berlaku: tahap asal `kuota − 1`, tahap tujuan `kuota + 1` dan `sisa_kuota + 1`.
   Jumlah kuota tahap tetap sama dengan kuota kategori (BR-04 tetap benar), dan kursinya bisa dijual lagi
   dengan harga tahap tujuan.
3. Jika semua tahap sesudahnya juga sudah lewat, kuota kembali ke tahap asal.

Tahap yang kuotanya menjadi 0 karena pemindahan ini disembunyikan di tampilan.
Kolom `tickets.price_tier_id` tidak berubah, karena mencatat harga yang dulu dibayar.
Aturan ini ada di satu fungsi bersama (`event.KembalikanKuota`), dipakai pesanan kedaluwarsa dan pembatalan tiket.

**Opsi yang tidak dipilih (Opsi A).** Selalu kembali ke tahap asal, sesuai teks BR-10 apa adanya.
Lebih sederhana, tetapi kursi dari tahap yang sudah lewat menjadi hangus.

### Keputusan: pembatalan tiket, pembatalan event, dan refund (BR-17 s.d. BR-20)
1. **Siapa yang boleh membatalkan tiket.** Hanya pembeli pemilik pesanan. Refund dicatat atas nama
   pemilik pesanan (`refunds.user_id`), bukan atas nama pemegang tiket yang tertulis di tiket.
2. **Batas H-7 (BR-17) dihitung per tanggal kalender Asia/Jakarta**, jam diabaikan.
   Pembatalan boleh jika `tanggal hari ini ≤ tanggal event − 7 hari`.
   Contoh: event 20 Des pukul 19:00 → hari terakhir pembatalan 13 Des (sampai 23:59:59 WIB).
3. **Besar refund** dihitung dari `harga_bayar` tiket (setelah potongan promo), dibulatkan ke bawah:
   75% untuk pembatalan oleh pembeli (BR-18), 100% untuk event yang dibatalkan (BR-20).
4. **Pemakaian promo tidak dikembalikan**, baik pada pembatalan oleh pembeli maupun pembatalan event.
   Pemakaian promo hanya dikembalikan saat pesanan `EXPIRED` (BR-15).
5. **Status setelah pembatalan.**
   - Tiket `ACTIVE` yang dibatalkan pembeli atau terkena pembatalan event menjadi `REFUNDED`
     (ada baris di tabel `refunds`). Pesanannya tetap `PAID`.
   - Saat event dibatalkan, pesanan `HELD` menjadi `CANCELLED` dan tiketnya `CANCELLED` (belum dibayar,
     jadi tanpa refund). Tiket yang sudah `REFUNDED`, `CANCELLED`, atau `USED` tidak diubah.
6. **Kuota.** Tiket yang dibatalkan pembeli mengembalikan kuota lewat aturan pengembalian di atas (BR-19),
   jadi bisa dijual lagi. Pembatalan event tidak mengubah kuota karena penjualan ditutup.
7. **Kapan event boleh dibatalkan.** Hanya event `OPEN` yang belum dimulai (`tanggal_event > sekarang`).
   Event `DRAFT` cukup dihapus karena belum ada pesanan.

### Keputusan: pemindahan tiket (BR-21 s.d. BR-23)
1. **Siapa yang boleh memindahkan.** Hanya pembeli pemilik pesanan. Pemindahan hanya mengganti nama pemegang
   dan kode tiket; pesanan dan pemiliknya tidak berubah.
2. **Kode lama (BR-22)** disimpan di `ticket_transfers.kode_lama`. Saat pemeriksaan di pintu, pencarian
   mendahulukan `tickets.kode_tiket` (kode yang berlaku), baru `ticket_transfers.kode_lama`, supaya kode lama
   bisa dijelaskan sebagai "sudah tidak berlaku" dan kode yang berlaku selalu menang.
3. **Syarat (BR-23):** tiket `ACTIVE` (belum dipakai masuk), event `OPEN`, dan event belum dimulai
   (`tanggal_event > sekarang`). Pemindahan tidak terikat batas H-7 seperti pembatalan.
4. **Tiket yang sudah dipindahkan tidak bisa dibatalkan oleh pembeli** (cek tambahan pada BR-17).
   Jika event dibatalkan penyelenggara (BR-20), tiket tersebut tetap di-refund 100% ke pemilik pesanan.
