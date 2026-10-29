package event

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// ===== Unit: ke tahap mana kuota dikembalikan =====

func TestTahapTujuanPengembalian(t *testing.T) {
	// tigaTahap: Presale 1 berakhir +5 hari, Presale 2 +10 hari, Normal +20 hari dari "sekarang"
	kasus := []struct {
		nama   string
		asalID int64
		now    time.Time
		mauID  int64
	}{
		{"tahap asal masih berlaku -> kembali ke asal", 1, sekarang, 1},
		{"tepat di batas tanggal asal -> masih kembali ke asal", 1, sekarang.Add(hari(5)), 1},
		{"asal lewat -> pindah ke tahap berikutnya", 1, sekarang.Add(hari(6)), 2},
		{"asal dan berikutnya lewat -> pindah ke tahap ketiga", 1, sekarang.Add(hari(11)), 3},
		{"semua tahap lewat -> kembali ke asal", 1, sekarang.Add(hari(21)), 1},
		{"tahap terakhir lewat -> tidak ada tahap sesudahnya, kembali ke asal", 3, sekarang.Add(hari(21)), 3},
		{"tidak pindah ke tahap sebelumnya", 2, sekarang.Add(hari(11)), 3},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if got := TahapTujuanPengembalian(tigaTahap(), k.asalID, k.now); got != k.mauID {
				t.Errorf("dapat tahap %d, mau %d", got, k.mauID)
			}
		})
	}
}

func TestSisaTersedia(t *testing.T) {
	tahap := tigaTahap() // sisa 10 + 10 + 10
	if got := SisaTersedia(tahap, sekarang); got != 30 {
		t.Errorf("semua berlaku: dapat %d, mau 30", got)
	}
	// Presale 1 sudah lewat: sisanya tidak bisa dipesan lagi
	if got := SisaTersedia(tahap, sekarang.Add(hari(6))); got != 20 {
		t.Errorf("Presale 1 lewat: dapat %d, mau 20", got)
	}
}

// ===== Integrasi: KembalikanKuota di database =====

// siapkanKategori membuat event dengan satu kategori dan tahap-tahap yang batas tanggalnya
// ditentukan per test (boleh sudah lewat), lalu mensimulasikan 1 tiket terambil dari tahap asal.
type tahapUji struct {
	nama     string
	kuota    int
	hariLagi int // negatif berarti batas tanggal sudah lewat
}

func siapkanKategori(t *testing.T, db *sql.DB, daftar []tahapUji, indeksAsal int) (kategoriID int64, tahapID []int64) {
	t.Helper()
	total := 0
	for _, d := range daftar {
		total += d.kuota
	}
	eventID := buatEventDraft(t, db, total)
	kategoriID = harusTambahKategori(t, db, eventID, "Festival", total)
	for _, d := range daftar {
		tahapID = append(tahapID, harusTambahTahap(t, db, eventID, kategoriID, d.nama, d.kuota, d.hariLagi))
	}
	// 1 tiket terjual dari tahap asal
	if _, err := db.Exec("UPDATE price_tiers SET sisa_kuota = sisa_kuota - 1 WHERE id = ?", tahapID[indeksAsal]); err != nil {
		t.Fatal(err)
	}
	return kategoriID, tahapID
}

// kembalikan menjalankan KembalikanKuota dengan urutan kunci yang benar, lalu commit
func kembalikan(t *testing.T, db *sql.DB, kategoriID, asalID int64) int64 {
	t.Helper()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := KunciTahapKategori(ctx, tx, []int64{kategoriID}); err != nil {
		t.Fatal(err)
	}
	tujuan, err := KembalikanKuota(ctx, tx, kategoriID, asalID, time.Now())
	if err != nil {
		t.Fatalf("KembalikanKuota: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return tujuan
}

type kuotaSisa struct{ kuota, sisa int }

func bacaTahap(t *testing.T, db *sql.DB, id int64) kuotaSisa {
	t.Helper()
	var k kuotaSisa
	if err := db.QueryRow("SELECT kuota, sisa_kuota FROM price_tiers WHERE id = ?", id).Scan(&k.kuota, &k.sisa); err != nil {
		t.Fatal(err)
	}
	return k
}

// jumlahKuotaTahapSamaDenganKategori memastikan BR-04 tetap benar setelah kuota dipindah
func jumlahKuotaTahapSamaDenganKategori(t *testing.T, db *sql.DB, kategoriID int64) {
	t.Helper()
	var kuotaKategori, totalTahap int
	db.QueryRow("SELECT kuota FROM ticket_categories WHERE id = ?", kategoriID).Scan(&kuotaKategori)
	db.QueryRow("SELECT SUM(kuota) FROM price_tiers WHERE category_id = ? AND deleted_at IS NULL", kategoriID).Scan(&totalTahap)
	if kuotaKategori != totalTahap {
		t.Errorf("BR-04 rusak: total kuota tahap %d, kuota kategori %d", totalTahap, kuotaKategori)
	}
}

func TestKembalikanKuotaKeTahapAsalJikaMasihBerlaku(t *testing.T) {
	db := bukaDBKosong(t)
	kategoriID, id := siapkanKategori(t, db, []tahapUji{{"Presale 1", 10, 5}, {"Normal", 10, 20}}, 0)

	if tujuan := kembalikan(t, db, kategoriID, id[0]); tujuan != id[0] {
		t.Errorf("tujuan %d, mau tahap asal %d", tujuan, id[0])
	}
	if got := bacaTahap(t, db, id[0]); got != (kuotaSisa{10, 10}) {
		t.Errorf("Presale 1: %+v, mau kuota 10 sisa 10", got)
	}
	if got := bacaTahap(t, db, id[1]); got != (kuotaSisa{10, 10}) {
		t.Errorf("Normal tidak boleh berubah: %+v", got)
	}
	jumlahKuotaTahapSamaDenganKategori(t, db, kategoriID)
}

func TestKembalikanKuotaPindahKeTahapBerikutnyaJikaAsalLewat(t *testing.T) {
	db := bukaDBKosong(t)
	// Presale 1 sudah lewat kemarin, Presale 2 masih berlaku
	kategoriID, id := siapkanKategori(t, db, []tahapUji{{"Presale 1", 10, -1}, {"Presale 2", 10, 5}, {"Normal", 10, 20}}, 0)

	if tujuan := kembalikan(t, db, kategoriID, id[0]); tujuan != id[1] {
		t.Errorf("tujuan %d, mau Presale 2 (%d)", tujuan, id[1])
	}
	// Presale 1: kuota 10 -> 9, sisa tetap 9 (kursinya tidak kembali ke tahap yang sudah lewat)
	if got := bacaTahap(t, db, id[0]); got != (kuotaSisa{9, 9}) {
		t.Errorf("Presale 1: %+v, mau kuota 9 sisa 9", got)
	}
	// Presale 2: kuota 10 -> 11, sisa 10 -> 11 (kursinya bisa dijual lagi)
	if got := bacaTahap(t, db, id[1]); got != (kuotaSisa{11, 11}) {
		t.Errorf("Presale 2: %+v, mau kuota 11 sisa 11", got)
	}
	if got := bacaTahap(t, db, id[2]); got != (kuotaSisa{10, 10}) {
		t.Errorf("Normal tidak boleh berubah: %+v", got)
	}
	jumlahKuotaTahapSamaDenganKategori(t, db, kategoriID)
}

func TestKembalikanKuotaKeAsalJikaSemuaTahapLewat(t *testing.T) {
	db := bukaDBKosong(t)
	kategoriID, id := siapkanKategori(t, db, []tahapUji{{"Presale 1", 10, -3}, {"Normal", 10, -1}}, 0)

	if tujuan := kembalikan(t, db, kategoriID, id[0]); tujuan != id[0] {
		t.Errorf("tujuan %d, mau tahap asal %d", tujuan, id[0])
	}
	if got := bacaTahap(t, db, id[0]); got != (kuotaSisa{10, 10}) {
		t.Errorf("Presale 1: %+v, mau kuota 10 sisa 10", got)
	}
	jumlahKuotaTahapSamaDenganKategori(t, db, kategoriID)
}

func TestKembalikanKuotaTahapBerkuotaSatu(t *testing.T) {
	db := bukaDBKosong(t)
	// Presale 1 hanya 1 kursi, sudah terjual, dan tanggalnya sudah lewat
	kategoriID, id := siapkanKategori(t, db, []tahapUji{{"Presale 1", 1, -1}, {"Normal", 10, 20}}, 0)

	if tujuan := kembalikan(t, db, kategoriID, id[0]); tujuan != id[1] {
		t.Errorf("tujuan %d, mau Normal (%d)", tujuan, id[1])
	}
	// kuota Presale 1 menjadi 0 (diizinkan migrasi 006) dan disembunyikan di tampilan
	if got := bacaTahap(t, db, id[0]); got != (kuotaSisa{0, 0}) {
		t.Errorf("Presale 1: %+v, mau kuota 0 sisa 0", got)
	}
	if got := bacaTahap(t, db, id[1]); got != (kuotaSisa{11, 11}) {
		t.Errorf("Normal: %+v, mau kuota 11 sisa 11", got)
	}
	jumlahKuotaTahapSamaDenganKategori(t, db, kategoriID)

	kategori, err := AmbilKategoriDanTahap(context.Background(), db, mustEventIDKategori(t, db, kategoriID))
	if err != nil {
		t.Fatal(err)
	}
	if terlihat := TahapTerlihat(kategori[0].Tahap); len(terlihat) != 1 || terlihat[0].ID != id[1] {
		t.Errorf("tahap terlihat %+v, mau hanya Normal", terlihat)
	}
}

func mustEventIDKategori(t *testing.T, db *sql.DB, kategoriID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow("SELECT event_id FROM ticket_categories WHERE id = ?", kategoriID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
