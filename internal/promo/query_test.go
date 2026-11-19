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

func TestEditPromoMengubahDataDanKategoriTanpaMengubahPemakaian(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")
	umum := buatKategori(t, db, "Seminar", "DRAFT", "Umum")
	id, err := simpanPromo(ctx, db, Promo{
		Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{vip},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := denganTx(t, db, func(tx *sql.Tx) error { return PakaiPromo(ctx, tx, id) }); err != nil {
			t.Fatal(err)
		}
	}

	err = ubahPromo(ctx, db, Promo{
		ID: id, Kode: "BARU20", Persen: 20, KuotaPemakaian: 5, KategoriID: []int64{umum},
	})
	if err != nil {
		t.Fatal(err)
	}

	hasil, err := ambilPromo(ctx, db, id)
	if err != nil {
		t.Fatal(err)
	}
	if hasil.Kode != "BARU20" || hasil.Persen != 20 || hasil.KuotaPemakaian != 5 || hasil.Terpakai != 2 {
		t.Errorf("data promo setelah edit tidak sesuai: %+v", hasil)
	}
	if len(hasil.KategoriID) != 1 || hasil.KategoriID[0] != umum {
		t.Errorf("kategori setelah edit %v, mau [%d]", hasil.KategoriID, umum)
	}
}

func TestEditPromoDitolakTanpaMengubahDataJikaKuotaKodeAtauKategoriTidakValid(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")
	umum := buatKategori(t, db, "Seminar", "DRAFT", "Umum")
	batal := buatKategori(t, db, "Konser Batal", "CANCELLED", "Batal")
	id, err := simpanPromo(ctx, db, Promo{
		Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{vip},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := denganTx(t, db, func(tx *sql.Tx) error { return PakaiPromo(ctx, tx, id) }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := simpanPromo(ctx, db, Promo{
		Kode: "PROMO-LAIN", Persen: 10, KuotaPemakaian: 5, KategoriID: []int64{umum},
	}); err != nil {
		t.Fatal(err)
	}

	kasus := []struct {
		nama string
		p    Promo
		err  error
	}{
		{"kuota kurang dari terpakai", Promo{ID: id, Kode: "BARU20", Persen: 20, KuotaPemakaian: 1, KategoriID: []int64{umum}}, nil},
		{"kode milik promo lain", Promo{ID: id, Kode: "PROMO-LAIN", Persen: 20, KuotaPemakaian: 5, KategoriID: []int64{umum}}, ErrKodeTerdaftar},
		{"kategori event dibatalkan", Promo{ID: id, Kode: "BARU20", Persen: 20, KuotaPemakaian: 5, KategoriID: []int64{batal}}, nil},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			err := ubahPromo(ctx, db, k.p)
			if k.err != nil {
				if !errors.Is(err, k.err) {
					t.Errorf("dapat %v, mau %v", err, k.err)
				}
			} else {
				var salah KesalahanInput
				if !errors.As(err, &salah) {
					t.Errorf("mau KesalahanInput, dapat %v", err)
				}
			}

			hasil, errBaca := ambilPromo(ctx, db, id)
			if errBaca != nil {
				t.Fatal(errBaca)
			}
			if hasil.Kode != "HEMAT15" || hasil.Persen != 15 || hasil.KuotaPemakaian != 10 || hasil.Terpakai != 2 ||
				len(hasil.KategoriID) != 1 || hasil.KategoriID[0] != vip {
				t.Errorf("data berubah setelah edit ditolak: %+v", hasil)
			}
		})
	}
}

func TestEditPromoYangSudahDihapusTidakDitemukan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")
	id, err := simpanPromo(ctx, db, Promo{
		Kode: "HEMAT15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{vip},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := hapusPromo(ctx, db, id); err != nil {
		t.Fatal(err)
	}

	if _, err := ambilPromo(ctx, db, id); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("ambil promo dihapus: dapat %v, mau ErrTidakDitemukan", err)
	}
	if err := ubahPromo(ctx, db, Promo{
		ID: id, Kode: "BARU20", Persen: 20, KuotaPemakaian: 5, KategoriID: []int64{vip},
	}); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("edit promo dihapus: dapat %v, mau ErrTidakDitemukan", err)
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
