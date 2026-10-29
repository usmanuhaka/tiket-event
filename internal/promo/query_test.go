package promo

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
)

func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.KosongkanSemua(t, db)
	return db
}

// buatKategori membuat event dengan status tertentu dan satu kategori, mengembalikan id kategori
func buatKategori(t *testing.T, db *sql.DB, namaEvent, status, namaKategori string) int64 {
	t.Helper()
	res, err := db.Exec(
		"INSERT INTO events (nama, lokasi, tanggal_event, kapasitas, status) VALUES (?, 'Aula', ?, 100, ?)",
		namaEvent, time.Now().Add(30*24*time.Hour), status)
	if err != nil {
		t.Fatal(err)
	}
	eventID, _ := res.LastInsertId()
	res, err = db.Exec("INSERT INTO ticket_categories (event_id, nama, kuota) VALUES (?, ?, 100)", eventID, namaKategori)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// denganTx menjalankan f di dalam transaksi lalu commit
func denganTx(t *testing.T, db *sql.DB, f func(tx *sql.Tx) error) error {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func terpakai(t *testing.T, db *sql.DB, id int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT terpakai FROM promo_codes WHERE id = ?", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBR13SimpanPromoDenganBeberapaKategoriDariBeberapaEvent(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")
	umum := buatKategori(t, db, "Seminar", "DRAFT", "Umum")

	id, err := simpanPromo(ctx, db, Promo{Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{vip, umum, vip}})
	if err != nil {
		t.Fatal(err)
	}

	daftar, err := ambilSemuaPromo(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(daftar) != 1 || daftar[0].ID != id || len(daftar[0].KategoriID) != 2 {
		t.Fatalf("dapat %+v, mau 1 promo dengan 2 kategori (id dobel dibuang)", daftar)
	}
	if daftar[0].NamaKategori[0] != "Konser - VIP" {
		t.Errorf("nama kategori %q", daftar[0].NamaKategori[0])
	}

	// KunciPromo menemukan promo lewat kode (huruf kecil pun boleh) beserta kategorinya
	err = denganTx(t, db, func(tx *sql.Tx) error {
		p, err := KunciPromo(ctx, tx, "hemat15")
		if err != nil {
			return err
		}
		if !p.BerlakuUntuk(vip) || !p.BerlakuUntuk(umum) || p.Persen != 15 {
			t.Errorf("promo terkunci tidak sesuai: %+v", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBR13KodeGandaDitolakWalaupunPromoLamaSudahDihapus(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")

	id, err := simpanPromo(ctx, db, Promo{Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{vip}})
	if err != nil {
		t.Fatal(err)
	}
	if err := hapusPromo(ctx, db, id); err != nil {
		t.Fatal(err)
	}

	_, err = simpanPromo(ctx, db, Promo{Kode: NormalisasiKode("hemat15"), Persen: 20, KuotaPemakaian: 5, KategoriID: []int64{vip}})
	if !errors.Is(err, ErrKodeTerdaftar) {
		t.Errorf("mau ErrKodeTerdaftar, dapat %v", err)
	}
}

func TestKategoriTidakValidDitolakTanpaMenyimpanPromo(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	batal := buatKategori(t, db, "Konser Batal", "CANCELLED", "VIP")

	for _, kategori := range [][]int64{{999999}, {batal}} {
		_, err := simpanPromo(ctx, db, Promo{Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: kategori})
		var salah KesalahanInput
		if !errors.As(err, &salah) {
			t.Errorf("kategori %v: mau KesalahanInput, dapat %v", kategori, err)
		}
	}
	var n int
	db.QueryRow("SELECT COUNT(*) FROM promo_codes").Scan(&n)
	if n != 0 {
		t.Errorf("ada %d promo tersimpan padahal kategorinya tidak valid", n)
	}

	if pilihan, _ := ambilPilihanKategori(ctx, db); len(pilihan) != 0 {
		t.Errorf("kategori event CANCELLED muncul di pilihan: %+v", pilihan)
	}
}

func TestPromoDihapusTidakBisaDipakaiTapiPemakaianTetapBisaDikembalikan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")
	id, err := simpanPromo(ctx, db, Promo{Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{vip}})
	if err != nil {
		t.Fatal(err)
	}

	// dipakai satu pesanan, lalu admin menghapus promo
	if err := denganTx(t, db, func(tx *sql.Tx) error { return PakaiPromo(ctx, tx, id) }); err != nil {
		t.Fatal(err)
	}
	if err := hapusPromo(ctx, db, id); err != nil {
		t.Fatal(err)
	}

	// pesanan baru tidak bisa memakai kode yang sudah dihapus
	err = denganTx(t, db, func(tx *sql.Tx) error {
		_, err := KunciPromo(ctx, tx, "HEMAT15")
		return err
	})
	if !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("promo dihapus masih bisa dikunci: %v", err)
	}
	if daftar, _ := ambilSemuaPromo(ctx, db); len(daftar) != 0 {
		t.Errorf("promo dihapus masih tampil di daftar")
	}

	// tetapi pemakaian dari pesanan lama tetap bisa dikembalikan (dikunci lewat id)
	if err := denganTx(t, db, func(tx *sql.Tx) error { return KembalikanPemakaian(ctx, tx, id) }); err != nil {
		t.Fatalf("kembalikan pemakaian promo yang sudah dihapus: %v", err)
	}
	if n := terpakai(t, db, id); n != 0 {
		t.Errorf("terpakai %d, mau 0", n)
	}
}
