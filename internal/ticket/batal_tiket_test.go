package ticket

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/event"
)

func TestBR17BR18BR19BatalTepatWaktu(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)

	// pesanan 2 tiket memakai promo 15%: harga 150.000, potongan 22.500, dibayar 127.500
	res, _ := db.Exec("INSERT INTO promo_codes (kode, persen, kuota_pemakaian, terpakai) VALUES ('HEMAT15', 15, 5, 1)")
	promoID, _ := res.LastInsertId()
	p := buatPesanan(t, db, userID, e, "PAID", 2, 0, 22500, sql.NullInt64{Int64: promoID, Valid: true})

	hasil, err := batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// BR-18: 75% dari 127.500 = 95.625
	if hasil.Refund != 95625 || hasil.OrderID != p.orderID {
		t.Errorf("hasil %+v, mau refund 95625 untuk pesanan %d", hasil, p.orderID)
	}
	if s := teks(t, db, "SELECT status FROM tickets WHERE id = ?", p.tiketID[0]); s != tiketRefunded {
		t.Errorf("status tiket %s, mau REFUNDED", s)
	}
	if s := teks(t, db, "SELECT status FROM tickets WHERE id = ?", p.tiketID[1]); s != tiketActive {
		t.Errorf("tiket lain di pesanan ikut berubah: %s", s)
	}
	if s := teks(t, db, "SELECT status FROM orders WHERE id = ?", p.orderID); s != "PAID" {
		t.Errorf("status pesanan %s, mau tetap PAID", s)
	}
	// refund dicatat atas nama pemilik pesanan
	var jumlah, penerima int64
	var persen int
	var alasan string
	db.QueryRow("SELECT jumlah, user_id, persen, alasan FROM refunds WHERE ticket_id = ?", p.tiketID[0]).Scan(&jumlah, &penerima, &persen, &alasan)
	if jumlah != 95625 || penerima != userID || persen != 75 || alasan != AlasanPembeli {
		t.Errorf("refund tersimpan: %d, user %d, %d%%, %s", jumlah, penerima, persen, alasan)
	}
	// BR-19: kuota kembali (10 - 2 + 1)
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 9 {
		t.Errorf("sisa kuota %d, mau 9", got.sisa)
	}
	// pemakaian promo tidak dikembalikan (PRD §9)
	if n := hitung(t, db, "SELECT terpakai FROM promo_codes WHERE id = ?", promoID); n != 1 {
		t.Errorf("terpakai promo %d, mau tetap 1", n)
	}
}

func TestBR17LewatH7DitolakTanpaPerubahan(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, time.Now().Add(hari(5)), []tahapUji{{"Normal", 150000, 10, 4}})
	userID := buatPembeli(t, db)
	p := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())

	_, err := batalkanTiket(context.Background(), db, p.tiketID[0], userID, time.Now())
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "paling lambat") {
		t.Fatalf("mau ditolak 'paling lambat', dapat %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM refunds"); n != 0 {
		t.Errorf("ada %d refund padahal ditolak", n)
	}
	if s := teks(t, db, "SELECT status FROM tickets WHERE id = ?", p.tiketID[0]); s != tiketActive {
		t.Errorf("status tiket %s, mau tetap ACTIVE", s)
	}
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 9 {
		t.Errorf("sisa kuota %d, mau tetap 9", got.sisa)
	}
}

// BR-19 + keputusan Opsi B: tiket dari tahap yang sudah lewat, kursinya pindah ke tahap berikutnya
func TestBR19KuotaPindahKeTahapBerlakuSehinggaBisaDipesanLagi(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	// Presale sudah berakhir kemarin, Normal masih berlaku
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Presale", 100000, 5, -1}, {"Normal", 150000, 5, 20}})
	userID := buatPembeli(t, db)
	p := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())

	kategoriSebelum, _ := event.AmbilKategoriDanTahap(ctx, db, e.eventID)
	tersediaSebelum := event.SisaTersedia(kategoriSebelum[0].Tahap, time.Now())

	if _, err := batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now()); err != nil {
		t.Fatal(err)
	}

	if got := bacaTahap(t, db, e.tahapID[0]); got != (kuotaSisa{4, 4}) {
		t.Errorf("Presale: %+v, mau kuota 4 sisa 4", got)
	}
	if got := bacaTahap(t, db, e.tahapID[1]); got != (kuotaSisa{6, 6}) {
		t.Errorf("Normal: %+v, mau kuota 6 sisa 6", got)
	}
	kategoriSesudah, _ := event.AmbilKategoriDanTahap(ctx, db, e.eventID)
	if tersedia := event.SisaTersedia(kategoriSesudah[0].Tahap, time.Now()); tersedia != tersediaSebelum+1 {
		t.Errorf("tiket yang bisa dipesan %d, mau %d (bertambah 1)", tersedia, tersediaSebelum+1)
	}
}

func TestBatalTiketDitolakUntukBukanPemilikDanTiketHeld(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	pemilik := buatPembeli(t, db)
	orangLain := buatPembeli(t, db)
	lunas := buatPesanan(t, db, pemilik, e, "PAID", 1, 0, 0, tanpaPromo())
	ditahan := buatPesanan(t, db, pemilik, e, "HELD", 1, 0, 0, tanpaPromo())

	if _, err := batalkanTiket(ctx, db, lunas.tiketID[0], orangLain, time.Now()); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("orang lain membatalkan: dapat %v, mau ErrTidakDitemukan", err)
	}
	var salah KesalahanInput
	if _, err := batalkanTiket(ctx, db, ditahan.tiketID[0], pemilik, time.Now()); !errors.As(err, &salah) {
		t.Errorf("tiket HELD: dapat %v, mau KesalahanInput", err)
	}
	if _, err := batalkanTiket(ctx, db, 999999, pemilik, time.Now()); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("tiket tidak ada: dapat %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM refunds"); n != 0 {
		t.Errorf("ada %d refund", n)
	}
}

func TestBatalDuaKaliBerurutan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	p := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())

	if _, err := batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now())
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "REFUNDED") {
		t.Errorf("batal kedua: dapat %v, mau ditolak (status REFUNDED)", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM refunds"); n != 1 {
		t.Errorf("jumlah refund %d, mau 1", n)
	}
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 10 {
		t.Errorf("sisa kuota %d, mau 10 (kembali sekali)", got.sisa)
	}
}

// Pembeli menekan "Batalkan" dua kali dengan cepat (dua request bersamaan untuk tiket yang sama).
// Diulang 10 putaran.
func TestBatalDuaKaliBersamaan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()

	for putaran := 1; putaran <= 10; putaran++ {
		e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
		userID := buatPembeli(t, db)
		p := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())

		var wg sync.WaitGroup
		var mu sync.Mutex
		berhasil, ditolak := 0, 0
		mulai := make(chan struct{})
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-mulai
				_, err := batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now())
				mu.Lock()
				defer mu.Unlock()
				var salah KesalahanInput
				switch {
				case err == nil:
					berhasil++
				case errors.As(err, &salah):
					ditolak++
				default:
					t.Errorf("putaran %d: error tak terduga %v", putaran, err)
				}
			}()
		}
		close(mulai)
		wg.Wait()

		if berhasil != 1 || ditolak != 1 {
			t.Fatalf("putaran %d: berhasil %d, ditolak %d; mau 1 dan 1", putaran, berhasil, ditolak)
		}
		if n := hitung(t, db, "SELECT COUNT(*) FROM refunds WHERE ticket_id = ?", p.tiketID[0]); n != 1 {
			t.Errorf("putaran %d: jumlah refund %d, mau 1", putaran, n)
		}
		if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 10 {
			t.Errorf("putaran %d: sisa kuota %d, mau 10 (kembali tepat sekali)", putaran, got.sisa)
		}
	}
}
