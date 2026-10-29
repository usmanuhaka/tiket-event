package order

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBR09PesanMengurangiKuotaDanHeld15Menit(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Presale 1", 100000, 10, 5}, {"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()

	orderID, err := buatPesanan(context.Background(), db, userID, e.eventID, "", pesan(e.kategoriID, 2), now)
	if err != nil {
		t.Fatal(err)
	}

	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 8 {
		t.Errorf("sisa Presale 1 = %d, mau 8", got.sisa)
	}
	p, err := ambilPesananMilik(context.Background(), db, orderID, userID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusHeld || !p.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Errorf("status %s, expires %v; mau HELD dan %v", p.Status, p.ExpiresAt, now.Add(15*time.Minute))
	}
	if len(p.Tiket) != 2 || p.Tiket[0].Status != TiketHeld || p.Tiket[0].KodeTiket != "" {
		t.Errorf("tiket tidak sesuai: %+v", p.Tiket)
	}
	if p.TotalHarga != 200000 || p.TotalBayar != 200000 {
		t.Errorf("total %d / bayar %d, mau 200000", p.TotalHarga, p.TotalBayar)
	}
}

func TestBR08AlokasiBerurutanTersimpanDiDatabase(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Presale 1", 100000, 1, 5}, {"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)

	// contoh PRD: tahap aktif tinggal 1 kuota, pesan 3
	orderID, err := buatPesanan(context.Background(), db, userID, e.eventID, "", pesan(e.kategoriID, 3), waktuSekarang())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ambilPesananMilik(context.Background(), db, orderID, userID)

	mau := []struct {
		tahap string
		harga int64
	}{{"Presale 1", 100000}, {"Normal", 150000}, {"Normal", 150000}}
	for i, m := range mau {
		if p.Tiket[i].NamaTahap != m.tahap || p.Tiket[i].Harga != m.harga {
			t.Errorf("tiket ke-%d: %s %d, mau %s %d", i+1, p.Tiket[i].NamaTahap, p.Tiket[i].Harga, m.tahap, m.harga)
		}
	}
	if p.TotalHarga != 400000 {
		t.Errorf("total %d, mau 400000", p.TotalHarga)
	}
	if bacaTahap(t, db, e.tahapID[0]).sisa != 0 || bacaTahap(t, db, e.tahapID[1]).sisa != 8 {
		t.Error("sisa kuota tahap tidak sesuai alokasi")
	}
}

func TestBR06PesananCampurKategori(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	var tahapVIP []int64
	vip := tambahKategoriUji(t, db, e.eventID, "VIP", []tahapUji{{"Normal", 400000, 5, 20}}, &tahapVIP)
	userID := buatPembeli(t, db)

	baris := []BarisTiket{
		{KategoriID: e.kategoriID, NamaPemegang: "Ani"},
		{KategoriID: vip, NamaPemegang: "Budi"},
		{KategoriID: e.kategoriID, NamaPemegang: "Cici"},
	}
	orderID, err := buatPesanan(context.Background(), db, userID, e.eventID, "", baris, waktuSekarang())
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ambilPesananMilik(context.Background(), db, orderID, userID)
	if p.TotalHarga != 700000 || p.Tiket[1].NamaKategori != "VIP" || p.Tiket[1].NamaPemegang != "Budi" {
		t.Errorf("pesanan campur kategori tidak sesuai: total %d, tiket %+v", p.TotalHarga, p.Tiket)
	}
}

// Semua penolakan tidak boleh meninggalkan perubahan apa pun di database
func TestPesananDitolakTanpaPerubahan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 3, 20}})
	lain := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 3, 20}})
	userID := buatPembeli(t, db)

	draft := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 3, 20}})
	db.Exec("UPDATE events SET status = 'DRAFT' WHERE id = ?", draft.eventID)
	lewat := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 3, 20}})
	db.Exec("UPDATE events SET tanggal_event = ? WHERE id = ?", time.Now().Add(-time.Hour), lewat.eventID)

	kasus := []struct {
		nama    string
		eventID int64
		baris   []BarisTiket
	}{
		{"kuota kurang (BR-12)", e.eventID, pesan(e.kategoriID, 4)},
		{"kategori milik event lain (BR-06)", e.eventID, pesan(lain.kategoriID, 1)},
		{"lima tiket (BR-06)", e.eventID, pesan(e.kategoriID, 5)},
		{"nama pemegang kosong (BR-07)", e.eventID, []BarisTiket{{KategoriID: e.kategoriID, NamaPemegang: " "}}},
		{"event masih DRAFT", draft.eventID, pesan(draft.kategoriID, 1)},
		{"event sudah berlangsung", lewat.eventID, pesan(lewat.kategoriID, 1)},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := buatPesanan(ctx, db, userID, k.eventID, "", k.baris, waktuSekarang())
			var salah KesalahanInput
			if !errors.As(err, &salah) {
				t.Errorf("mau KesalahanInput, dapat %v", err)
			}
		})
	}

	if n := hitung(t, db, "SELECT COUNT(*) FROM orders"); n != 0 {
		t.Errorf("ada %d pesanan tersimpan padahal semua ditolak", n)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM price_tiers WHERE sisa_kuota <> kuota"); n != 0 {
		t.Errorf("ada %d tahap yang kuotanya berubah", n)
	}

	if _, err := buatPesanan(ctx, db, userID, 999999, "", pesan(e.kategoriID, 1), waktuSekarang()); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("event tidak ada: dapat %v", err)
	}
}

func TestPesananHanyaTerlihatOlehPemiliknya(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	pemilik := buatPembeli(t, db)
	orangLain := buatPembeli(t, db)

	orderID, err := buatPesanan(ctx, db, pemilik, e.eventID, "", pesan(e.kategoriID, 1), waktuSekarang())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ambilPesananMilik(ctx, db, orderID, orangLain); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("pesanan terlihat oleh orang lain: %v", err)
	}
	daftar, err := ambilPesananUser(ctx, db, pemilik)
	if err != nil || len(daftar) != 1 {
		t.Errorf("daftar pesanan pemilik: %d, err %v", len(daftar), err)
	}
	if daftar, _ := ambilPesananUser(ctx, db, orangLain); len(daftar) != 0 {
		t.Errorf("orang lain melihat %d pesanan", len(daftar))
	}
}
