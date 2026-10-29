package order

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/promo"
)

// ===== Unit: potongan per tiket (BR-14) =====

func TestPotonganPesananHanyaUntukKategoriTerdaftar(t *testing.T) {
	baris := []BarisTiket{{KategoriID: 1}, {KategoriID: 2}, {KategoriID: 1}}
	tiket := []Alokasi{{Harga: 150000}, {Harga: 99999}, {Harga: 175000}}
	p := promo.Promo{Kode: "VIP10", Persen: 10, KategoriID: []int64{2}}

	potongan, err := PotonganPesanan(baris, tiket, p)
	if err != nil {
		t.Fatal(err)
	}
	mau := []int64{0, 9999, 0} // hanya kategori 2; 9.999,9 dibulatkan ke bawah
	for i := range mau {
		if potongan[i] != mau[i] {
			t.Errorf("tiket ke-%d: potongan %d, mau %d", i+1, potongan[i], mau[i])
		}
	}
}

func TestPotonganPesananDitolakJikaTidakAdaTiketYangBerlaku(t *testing.T) {
	baris := []BarisTiket{{KategoriID: 1}}
	p := promo.Promo{Kode: "VIP10", Persen: 10, KategoriID: []int64{2}}
	_, err := PotonganPesanan(baris, []Alokasi{{Harga: 150000}}, p)
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "tidak berlaku") {
		t.Errorf("mau ditolak 'tidak berlaku', dapat %v", err)
	}
}

// ===== Integrasi =====

// buatPromo menyimpan promo untuk kategori tertentu langsung lewat SQL, mengembalikan id-nya
func buatPromo(t *testing.T, db *sql.DB, kode string, persen, kuota int, kategoriID ...int64) int64 {
	t.Helper()
	res, err := db.Exec("INSERT INTO promo_codes (kode, persen, kuota_pemakaian) VALUES (?, ?, ?)", kode, persen, kuota)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	for _, k := range kategoriID {
		if _, err := db.Exec("INSERT INTO promo_categories (promo_code_id, category_id) VALUES (?, ?)", id, k); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func terpakai(t *testing.T, db *sql.DB, promoID int64) int {
	t.Helper()
	return hitung(t, db, "SELECT terpakai FROM promo_codes WHERE id = ?", promoID)
}

// eventFestivalVIP: Festival Rp150.000 (kuota 10) dan VIP Rp400.000 (kuota 5)
func eventFestivalVIP(t *testing.T, db *sql.DB) (e eventUji, vip int64) {
	t.Helper()
	e = siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	var tahapVIP []int64
	vip = tambahKategoriUji(t, db, e.eventID, "VIP", []tahapUji{{"Normal", 400000, 5, 20}}, &tahapVIP)
	return e, vip
}

func TestBR14PotonganTersimpanPerTiketHanyaUntukKategoriPromo(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e, vip := eventFestivalVIP(t, db)
	buatPromo(t, db, "VIP15", 15, 10, vip)
	userID := buatPembeli(t, db)

	baris := []BarisTiket{
		{KategoriID: e.kategoriID, NamaPemegang: "Ani"},
		{KategoriID: vip, NamaPemegang: "Budi"},
		{KategoriID: e.kategoriID, NamaPemegang: "Cici"},
	}
	orderID, err := buatPesanan(ctx, db, userID, e.eventID, "vip15", baris, waktuSekarang()) // huruf kecil pun boleh
	if err != nil {
		t.Fatal(err)
	}

	p, _ := ambilPesananMilik(ctx, db, orderID, userID)
	// VIP: 15% x 400.000 = 60.000; Festival tanpa potongan
	mau := []struct{ harga, potongan, bayar int64 }{
		{150000, 0, 150000},
		{400000, 60000, 340000},
		{150000, 0, 150000},
	}
	for i, m := range mau {
		tk := p.Tiket[i]
		if tk.Harga != m.harga || tk.Potongan != m.potongan || tk.HargaBayar != m.bayar {
			t.Errorf("tiket ke-%d: %d/%d/%d, mau %d/%d/%d", i+1, tk.Harga, tk.Potongan, tk.HargaBayar, m.harga, m.potongan, m.bayar)
		}
	}
	if p.TotalHarga != 700000 || p.TotalPotongan != 60000 || p.TotalBayar != 640000 || p.KodePromo != "VIP15" {
		t.Errorf("total %d, potongan %d, bayar %d, kode %q", p.TotalHarga, p.TotalPotongan, p.TotalBayar, p.KodePromo)
	}
}

func TestBR15PemakaianDihitungSaatPesanDanTetapSetelahBayar(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	promoID := buatPromo(t, db, "HEMAT10", 10, 5, e.kategoriID)
	userID := buatPembeli(t, db)
	now := waktuSekarang()

	// satu pesanan berisi 4 tiket tetap dihitung 1 pemakaian
	orderID, err := buatPesanan(ctx, db, userID, e.eventID, "HEMAT10", pesan(e.kategoriID, 4), now)
	if err != nil {
		t.Fatal(err)
	}
	if n := terpakai(t, db, promoID); n != 1 {
		t.Errorf("setelah pesan: terpakai %d, mau 1", n)
	}
	if err := bayarPesanan(ctx, db, orderID, userID, now); err != nil {
		t.Fatal(err)
	}
	if n := terpakai(t, db, promoID); n != 1 {
		t.Errorf("setelah bayar: terpakai %d, mau tetap 1", n)
	}
}

func TestBR15PemakaianKembaliSaatExpired(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	promoID := buatPromo(t, db, "HEMAT10", 10, 5, e.kategoriID)
	userID := buatPembeli(t, db)
	now := waktuSekarang()

	lewatWorker, _ := buatPesanan(ctx, db, userID, e.eventID, "HEMAT10", pesan(e.kategoriID, 1), now)
	lewatBayar, _ := buatPesanan(ctx, db, userID, e.eventID, "HEMAT10", pesan(e.kategoriID, 1), now)
	if n := terpakai(t, db, promoID); n != 2 {
		t.Fatalf("terpakai %d, mau 2", n)
	}

	// pesanan pertama: diproses worker; pesanan kedua: pembeli membayar terlambat
	if _, err := expirePesanan(ctx, db, lewatWorker, now.Add(16*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := terpakai(t, db, promoID); n != 1 {
		t.Errorf("setelah worker: terpakai %d, mau 1", n)
	}
	if err := bayarPesanan(ctx, db, lewatBayar, userID, now.Add(16*time.Minute)); !errors.Is(err, ErrWaktuBayarHabis) {
		t.Fatalf("bayar terlambat: %v", err)
	}
	if n := terpakai(t, db, promoID); n != 0 {
		t.Errorf("setelah bayar terlambat: terpakai %d, mau 0", n)
	}
}

// Tambahan dari review: admin menghapus promo selagi pesanan yang memakainya masih HELD
func TestBR15PemakaianPromoYangSudahDihapusTetapKembaliSaatExpired(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	promoID := buatPromo(t, db, "HEMAT10", 10, 5, e.kategoriID)
	userID := buatPembeli(t, db)
	now := waktuSekarang()

	orderID, err := buatPesanan(ctx, db, userID, e.eventID, "HEMAT10", pesan(e.kategoriID, 1), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE promo_codes SET deleted_at = NOW() WHERE id = ?", promoID); err != nil {
		t.Fatal(err)
	}

	// pesanan baru tidak bisa memakai kode yang sudah dihapus
	_, err = buatPesanan(ctx, db, userID, e.eventID, "HEMAT10", pesan(e.kategoriID, 1), now)
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "tidak ditemukan") {
		t.Errorf("kode yang dihapus masih bisa dipakai: %v", err)
	}

	// pesanan lama kedaluwarsa: pemakaiannya tetap dikembalikan
	jumlah, err := ExpireKedaluwarsa(ctx, db, now.Add(16*time.Minute))
	if err != nil || jumlah != 1 {
		t.Fatalf("worker: jumlah %d, err %v", jumlah, err)
	}
	if s := statusPesanan(t, db, orderID); s != StatusExpired {
		t.Errorf("status %s, mau EXPIRED", s)
	}
	if n := terpakai(t, db, promoID); n != 0 {
		t.Errorf("terpakai %d, mau 0", n)
	}
}

func TestBR16PromoHabisDitolakTanpaPerubahan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	promoID := buatPromo(t, db, "SEKALI", 50, 1, e.kategoriID)
	userID := buatPembeli(t, db)

	if _, err := buatPesanan(ctx, db, userID, e.eventID, "SEKALI", pesan(e.kategoriID, 1), waktuSekarang()); err != nil {
		t.Fatal(err)
	}
	_, err := buatPesanan(ctx, db, userID, e.eventID, "SEKALI", pesan(e.kategoriID, 2), waktuSekarang())
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "sudah habis") {
		t.Fatalf("mau ditolak 'sudah habis', dapat %v", err)
	}

	if n := terpakai(t, db, promoID); n != 1 {
		t.Errorf("terpakai %d, mau 1", n)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM orders"); n != 1 {
		t.Errorf("jumlah pesanan %d, mau 1", n)
	}
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 9 {
		t.Errorf("sisa tiket %d, mau 9 (pesanan yang ditolak tidak boleh mengurangi kuota)", got.sisa)
	}
}

func TestPromoTidakBerlakuAtauTidakAdaDitolakTanpaPemakaian(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e, vip := eventFestivalVIP(t, db)
	promoID := buatPromo(t, db, "VIP15", 15, 10, vip)
	userID := buatPembeli(t, db)

	for _, kode := range []string{"VIP15", "TIDAKADA"} {
		// hanya memesan Festival, sedangkan VIP15 hanya untuk VIP
		_, err := buatPesanan(ctx, db, userID, e.eventID, kode, pesan(e.kategoriID, 1), waktuSekarang())
		var salah KesalahanInput
		if !errors.As(err, &salah) {
			t.Errorf("kode %s: mau KesalahanInput, dapat %v", kode, err)
		}
	}
	if n := terpakai(t, db, promoID); n != 0 {
		t.Errorf("terpakai %d, mau 0", n)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM orders"); n != 0 {
		t.Errorf("ada %d pesanan tersimpan", n)
	}
}

// Dua pembeli menekan "Pesan" bersamaan dengan kode promo yang tinggal 1 kuota.
// Mereka memesan DUA EVENT BERBEDA (promo berlaku untuk kategori di keduanya), jadi kunci baris
// event tidak membuat mereka antre: yang menjaga BR-16 hanyalah kunci baris promo (FOR UPDATE).
// Diulang 10 putaran supaya urutan siapa yang menang ikut teruji.
func TestBR16DuaPembeliBerebutKuotaPromoTerakhir(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()

	for putaran := 1; putaran <= 10; putaran++ {
		eventA := siapkanEvent(t, db, []tahapUji{{"Normal", 100000, 10, 20}})
		eventB := siapkanEvent(t, db, []tahapUji{{"Normal", 100000, 10, 20}})
		kode := "TERAKHIR-" + string(rune('A'+putaran))
		promoID := buatPromo(t, db, kode, 50, 1, eventA.kategoriID, eventB.kategoriID)
		pembeli := []struct {
			userID int64
			e      eventUji
		}{{buatPembeli(t, db), eventA}, {buatPembeli(t, db), eventB}}

		var wg sync.WaitGroup
		var mu sync.Mutex
		berhasil, ditolak := 0, 0
		mulai := make(chan struct{})
		for _, p := range pembeli {
			wg.Add(1)
			go func(userID int64, e eventUji) {
				defer wg.Done()
				<-mulai
				_, err := buatPesanan(ctx, db, userID, e.eventID, kode, pesan(e.kategoriID, 2), waktuSekarang())
				mu.Lock()
				defer mu.Unlock()
				var salah KesalahanInput
				switch {
				case err == nil:
					berhasil++
				case errors.As(err, &salah) && strings.Contains(salah.Pesan, "sudah habis"):
					ditolak++
				default:
					t.Errorf("putaran %d: error tak terduga %v", putaran, err)
				}
			}(p.userID, p.e)
		}
		close(mulai)
		wg.Wait()

		if berhasil != 1 || ditolak != 1 {
			t.Fatalf("putaran %d: berhasil %d, ditolak %d; mau 1 dan 1", putaran, berhasil, ditolak)
		}
		if n := terpakai(t, db, promoID); n != 1 {
			t.Errorf("putaran %d: terpakai %d, mau 1", putaran, n)
		}
		// hanya pesanan yang berhasil (2 tiket) yang mengurangi kuota tiket: total sisa 10 + 8
		sisa := bacaTahap(t, db, eventA.tahapID[0]).sisa + bacaTahap(t, db, eventB.tahapID[0]).sisa
		if sisa != 18 {
			t.Errorf("putaran %d: total sisa tiket %d, mau 18", putaran, sisa)
		}
		// pesanan yang berhasil mendapat potongan 50% x 2 x Rp100.000
		if n := hitung(t, db, "SELECT COUNT(*) FROM orders WHERE promo_code_id = ? AND total_potongan = 100000", promoID); n != 1 {
			t.Errorf("putaran %d: pesanan berpromo dengan potongan benar = %d, mau 1", putaran, n)
		}
	}
}
