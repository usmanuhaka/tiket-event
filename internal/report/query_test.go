package report

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
	"github.com/usmanuhaka/tiket-event/internal/ticket"
)

func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.KosongkanSemua(t, db)
	return db
}

var nomor int

// data uji yang dibangun lewat SQL langsung

func buatPembeli(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	nomor++
	res, err := db.Exec("INSERT INTO users (nama, email, password_hash, role) VALUES ('Pembeli', ?, 'x', 'PEMBELI')",
		fmt.Sprintf("lap%d@kampus.test", nomor))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func buatEvent(t *testing.T, db *sql.DB, nama string) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO events (nama, lokasi, tanggal_event, kapasitas, status) VALUES (?, 'Aula', ?, 100, 'OPEN')",
		nama, time.Now().Add(30*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func buatKategori(t *testing.T, db *sql.DB, eventID int64, nama string) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO ticket_categories (event_id, nama, kuota) VALUES (?, ?, 20)", eventID, nama)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

func buatTahap(t *testing.T, db *sql.DB, kategoriID int64, nama string, urutan int, harga int64, kuota int) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO price_tiers (category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, kategoriID, nama, urutan, harga, kuota, kuota, time.Now().Add(20*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// buatTiket menyimpan satu pesanan berstatus statusPesanan dengan satu tiket berstatus statusTiket
func buatTiket(t *testing.T, db *sql.DB, userID, eventID, kategoriID, tahapID int64, harga, potongan int64, statusPesanan, statusTiket string) (orderID, tiketID int64) {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO orders (user_id, event_id, status, total_harga, total_potongan, total_bayar, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, userID, eventID, statusPesanan, harga, potongan, harga-potongan, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	orderID, _ = res.LastInsertId()
	res, err = db.Exec(
		`INSERT INTO tickets (order_id, category_id, price_tier_id, nama_pemegang, harga, potongan, harga_bayar, status)
		VALUES (?, ?, ?, 'Pemegang', ?, ?, ?, ?)`, orderID, kategoriID, tahapID, harga, potongan, harga-potongan, statusTiket)
	if err != nil {
		t.Fatal(err)
	}
	tiketID, _ = res.LastInsertId()
	return orderID, tiketID
}

// pembanding menghitung total event langsung dari tickets dan refunds, tanpa query laporan.
// Dipakai untuk memastikan query laporan tidak kehilangan atau menghitung dua kali baris mana pun.
func pembanding(t *testing.T, db *sql.DB, eventID int64) Angka {
	t.Helper()
	var a Angka
	err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(t.harga), 0), COALESCE(SUM(t.potongan), 0),
			COALESCE(SUM(CASE WHEN t.status = 'USED' THEN 1 ELSE 0 END), 0)
		FROM tickets t JOIN orders o ON o.id = t.order_id
		WHERE o.event_id = ? AND t.status IN ('ACTIVE', 'USED', 'REFUNDED')`, eventID,
	).Scan(&a.Terjual, &a.Kotor, &a.Potongan, &a.SudahMasuk)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(
		`SELECT COUNT(*), COALESCE(SUM(r.jumlah), 0) FROM refunds r JOIN orders o ON o.id = r.order_id WHERE o.event_id = ?`, eventID,
	).Scan(&a.TiketRefund, &a.Refund); err != nil {
		t.Fatal(err)
	}
	return a
}

// cekInvarian: jumlah per tahap = jumlah per kategori = total event = hitungan pembanding
func cekInvarian(t *testing.T, db *sql.DB, lap Laporan) {
	t.Helper()
	var dariTahap, dariKategori Angka
	for _, k := range lap.Kategori {
		var subtotal Angka
		for _, b := range k.Tahap {
			dariTahap.Tambah(b.Angka)
			subtotal.Tambah(b.Angka)
		}
		if subtotal != k.Subtotal {
			t.Errorf("kategori %s: jumlah tahap %+v != subtotal %+v", k.Nama, subtotal, k.Subtotal)
		}
		dariKategori.Tambah(k.Subtotal)
	}
	if dariTahap != lap.Total || dariKategori != lap.Total {
		t.Errorf("tahap %+v / kategori %+v / total %+v tidak sama", dariTahap, dariKategori, lap.Total)
	}
	if p := pembanding(t, db, lap.EventID); p != lap.Total {
		t.Errorf("total laporan %+v != pembanding langsung %+v", lap.Total, p)
	}
}

// Satu event dengan semua jenis status sekaligus:
//
//	Festival / Presale Rp150.000 : 2 ACTIVE berpromo 15% (potongan 22.500), 1 USED
//	Festival / Normal  Rp200.000 : 1 REFUNDED oleh pembeli (75% = 150.000), 1 HELD, 1 CANCELLED (pesanan EXPIRED)
//	VIP      / Normal  Rp400.000 : belum terjual
func TestBR27BR28LaporanPerTahapKategoriEvent(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	u := buatPembeli(t, db)
	eventID := buatEvent(t, db, "Konser")
	festival := buatKategori(t, db, eventID, "Festival")
	presale := buatTahap(t, db, festival, "Presale", 1, 150000, 10)
	normal := buatTahap(t, db, festival, "Normal", 2, 200000, 10)
	vip := buatKategori(t, db, eventID, "VIP")
	buatTahap(t, db, vip, "Normal", 1, 400000, 20)

	buatTiket(t, db, u, eventID, festival, presale, 150000, 22500, "PAID", "ACTIVE")
	buatTiket(t, db, u, eventID, festival, presale, 150000, 22500, "PAID", "ACTIVE")
	buatTiket(t, db, u, eventID, festival, presale, 150000, 0, "PAID", "USED")
	orderRefund, tiketRefund := buatTiket(t, db, u, eventID, festival, normal, 200000, 0, "PAID", "REFUNDED")
	db.Exec("INSERT INTO refunds (ticket_id, order_id, user_id, jumlah, persen, alasan) VALUES (?, ?, ?, ?, 75, 'PEMBELI')",
		tiketRefund, orderRefund, u, ticket.HitungRefund(200000, ticket.PersenRefundPembeli))
	buatTiket(t, db, u, eventID, festival, normal, 200000, 0, "HELD", "HELD")
	buatTiket(t, db, u, eventID, festival, normal, 200000, 0, "EXPIRED", "CANCELLED")

	lap, err := AmbilLaporan(ctx, db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	cekInvarian(t, db, lap)

	if len(lap.Kategori) != 2 || len(lap.Kategori[0].Tahap) != 2 || len(lap.Kategori[1].Tahap) != 1 {
		t.Fatalf("struktur laporan salah: %+v", lap.Kategori)
	}
	kasus := []struct {
		nama   string
		dapat  Angka
		mau    Angka
		bersih int64
	}{
		// promo: kotor tetap harga sebelum promo, potongan 2 x 22.500
		{"Festival/Presale", lap.Kategori[0].Tahap[0].Angka, Angka{Terjual: 3, SudahMasuk: 1, Kotor: 450000, Potongan: 45000}, 405000},
		// HELD dan CANCELLED tidak dihitung; pembatalan pembeli menyisakan 25% (200.000 - 150.000)
		{"Festival/Normal", lap.Kategori[0].Tahap[1].Angka, Angka{Terjual: 1, TiketRefund: 1, Kotor: 200000, Refund: 150000}, 50000},
		// tanpa penjualan: semua nol, tetapi tahapnya tetap tampil
		{"VIP/Normal", lap.Kategori[1].Tahap[0].Angka, Angka{}, 0},
		{"subtotal Festival", lap.Kategori[0].Subtotal, Angka{Terjual: 4, TiketRefund: 1, SudahMasuk: 1, Kotor: 650000, Potongan: 45000, Refund: 150000}, 455000},
		{"total event", lap.Total, Angka{Terjual: 4, TiketRefund: 1, SudahMasuk: 1, Kotor: 650000, Potongan: 45000, Refund: 150000}, 455000},
	}
	for _, k := range kasus {
		if k.dapat != k.mau || k.dapat.Bersih() != k.bersih {
			t.Errorf("%s: dapat %+v (bersih %d), mau %+v (bersih %d)", k.nama, k.dapat, k.dapat.Bersih(), k.mau, k.bersih)
		}
	}
}

func TestLaporanEventTanpaPenjualan(t *testing.T) {
	db := bukaDBKosong(t)
	eventID := buatEvent(t, db, "Sepi")
	k := buatKategori(t, db, eventID, "Umum")
	buatTahap(t, db, k, "Presale", 1, 50000, 10)
	buatTahap(t, db, k, "Normal", 2, 75000, 10)

	lap, err := AmbilLaporan(context.Background(), db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	cekInvarian(t, db, lap)
	if len(lap.Kategori) != 1 || len(lap.Kategori[0].Tahap) != 2 || lap.Total != (Angka{}) || lap.Total.Bersih() != 0 {
		t.Errorf("event tanpa penjualan: %+v", lap)
	}
}

// Tahap yang kuotanya sudah dipindah (kuota 0) tetap muncul jika pernah menjual tiket;
// tahap berkuota 0 tanpa penjualan disembunyikan
func TestLaporanTahapBerkuotaNolYangPernahMenjual(t *testing.T) {
	db := bukaDBKosong(t)
	u := buatPembeli(t, db)
	eventID := buatEvent(t, db, "Konser")
	k := buatKategori(t, db, eventID, "Festival")
	lama := buatTahap(t, db, k, "Presale Lama", 1, 100000, 1)
	kosong := buatTahap(t, db, k, "Presale Kosong", 2, 120000, 1)
	buatTahap(t, db, k, "Normal", 3, 150000, 10)
	buatTiket(t, db, u, eventID, k, lama, 100000, 0, "PAID", "ACTIVE")
	db.Exec("UPDATE price_tiers SET kuota = 0, sisa_kuota = 0 WHERE id IN (?, ?)", lama, kosong)

	lap, err := AmbilLaporan(context.Background(), db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	cekInvarian(t, db, lap)
	nama := []string{}
	for _, b := range lap.Kategori[0].Tahap {
		nama = append(nama, b.NamaTahap)
	}
	if fmt.Sprint(nama) != "[Presale Lama Normal]" {
		t.Errorf("tahap tampil %v, mau [Presale Lama Normal]", nama)
	}
}

// Laporan membaca satu snapshot: penjualan yang COMMIT setelah laporan mulai membaca
// tidak ikut terhitung di tengah laporan yang sama
func TestLaporanDibacaDariSatuSnapshot(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	u := buatPembeli(t, db)
	eventID := buatEvent(t, db, "Konser")
	k := buatKategori(t, db, eventID, "Festival")
	tahap := buatTahap(t, db, k, "Normal", 1, 100000, 10)
	buatTiket(t, db, u, eventID, k, tahap, 100000, 0, "PAID", "ACTIVE")

	// 1. laporan mulai: SELECT pertama menetapkan snapshot
	tx, err := db.BeginTx(ctx, opsiSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := bacaEvent(ctx, tx, eventID); err != nil {
		t.Fatal(err)
	}

	// 2. sementara itu, pembeli lain menyelesaikan pembayaran (koneksi lain, COMMIT)
	buatTiket(t, db, u, eventID, k, tahap, 100000, 0, "PAID", "ACTIVE")

	// 3. baris tahap di laporan yang sama tidak melihat penjualan baru
	baris, err := bacaBarisTahap(ctx, tx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if baris[0].Terjual != 1 || baris[0].Kotor != 100000 {
		t.Errorf("di dalam snapshot: terjual %d, kotor %d; mau 1 dan 100000", baris[0].Terjual, baris[0].Kotor)
	}
	tx.Commit()

	// 4. laporan berikutnya memuat penjualan itu
	lap, err := AmbilLaporan(ctx, db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if lap.Total.Terjual != 2 {
		t.Errorf("laporan baru: terjual %d, mau 2", lap.Total.Terjual)
	}
}

func TestLaporanEventTidakAda(t *testing.T) {
	db := bukaDBKosong(t)
	if _, err := AmbilLaporan(context.Background(), db, 999999); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("dapat %v, mau ErrTidakDitemukan", err)
	}
}

func TestRingkasanSemuaEventSamaDenganLaporanPerEvent(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	u := buatPembeli(t, db)
	for i := 1; i <= 2; i++ {
		eventID := buatEvent(t, db, fmt.Sprintf("Event %d", i))
		k := buatKategori(t, db, eventID, "Umum")
		tahap := buatTahap(t, db, k, "Normal", 1, int64(i)*100000, 10)
		for j := 0; j < i; j++ {
			buatTiket(t, db, u, eventID, k, tahap, int64(i)*100000, 10000, "PAID", "ACTIVE")
		}
		buatTiket(t, db, u, eventID, k, tahap, int64(i)*100000, 0, "HELD", "HELD")
	}
	draft := buatEvent(t, db, "Draft")
	db.Exec("UPDATE events SET status = 'DRAFT' WHERE id = ?", draft)

	ringkasan, err := ambilRingkasanSemuaEvent(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ringkasan) != 2 {
		t.Fatalf("jumlah event %d, mau 2 (DRAFT tidak tampil)", len(ringkasan))
	}
	for _, r := range ringkasan {
		lap, _ := AmbilLaporan(ctx, db, r.ID)
		if r.Angka != lap.Total {
			t.Errorf("event %s: ringkasan %+v != laporan %+v", r.Nama, r.Angka, lap.Total)
		}
	}
}
