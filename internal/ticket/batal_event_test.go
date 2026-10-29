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

// refundTiket membaca refund satu tiket: jumlah baris, persen, dan penerimanya
func refundTiket(t *testing.T, db *sql.DB, tiketID int64) (jumlahBaris int, persen int, nominal int64, penerima int64) {
	t.Helper()
	jumlahBaris = hitung(t, db, "SELECT COUNT(*) FROM refunds WHERE ticket_id = ?", tiketID)
	if jumlahBaris == 1 {
		db.QueryRow("SELECT persen, jumlah, user_id FROM refunds WHERE ticket_id = ?", tiketID).Scan(&persen, &nominal, &penerima)
	}
	return
}

func TestBR20BatalEventDenganBerbagaiStatusPesanan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 20, 20}})
	res, _ := db.Exec("INSERT INTO promo_codes (kode, persen, kuota_pemakaian, terpakai) VALUES ('HEMAT10', 10, 5, 2)")
	promoID, _ := res.LastInsertId()
	denganPromo := sql.NullInt64{Int64: promoID, Valid: true}
	u1, u2, u3, u4 := buatPembeli(t, db), buatPembeli(t, db), buatPembeli(t, db), buatPembeli(t, db)

	// A: PAID 2 tiket berpromo (dibayar 135.000 per tiket)
	a := buatPesanan(t, db, u1, e, "PAID", 2, 0, 15000, denganPromo)
	// B: PAID 2 tiket, satu sudah dibatalkan pembeli (refund 75%)
	b := buatPesanan(t, db, u2, e, "PAID", 2, 0, 0, tanpaPromo())
	if _, err := batalkanTiket(ctx, db, b.tiketID[0], u2, time.Now()); err != nil {
		t.Fatal(err)
	}
	// C: HELD berpromo
	c := buatPesanan(t, db, u3, e, "HELD", 1, 0, 15000, denganPromo)
	// D: EXPIRED (tiketnya CANCELLED)
	d := buatPesanan(t, db, u4, e, "HELD", 1, 0, 0, tanpaPromo())
	db.Exec("UPDATE orders SET status = 'EXPIRED' WHERE id = ?", d.orderID)
	db.Exec("UPDATE tickets SET status = 'CANCELLED' WHERE order_id = ?", d.orderID)
	// E: tiket sudah dipakai masuk (USED)
	ee := buatPesanan(t, db, u4, e, "PAID", 1, 0, 0, tanpaPromo())
	db.Exec("UPDATE tickets SET status = 'USED' WHERE id = ?", ee.tiketID[0])

	sisaSebelum := bacaTahap(t, db, e.tahapID[0])

	ringkasan, err := batalkanEvent(ctx, db, e.eventID, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// 2 tiket A (135.000) + 1 tiket B yang masih ACTIVE (150.000)
	if ringkasan != (RingkasanBatalEvent{TiketDirefund: 3, TotalRefund: 420000, PesananDibatalkan: 1}) {
		t.Errorf("ringkasan %+v", ringkasan)
	}

	// A: refund 100% dari harga_bayar, ke pemilik pesanan
	for _, id := range a.tiketID {
		if n, persen, nominal, penerima := refundTiket(t, db, id); n != 1 || persen != 100 || nominal != 135000 || penerima != u1 {
			t.Errorf("tiket A %d: %d refund, %d%%, Rp%d, ke user %d", id, n, persen, nominal, penerima)
		}
	}
	// B: tiket yang sudah dibatalkan pembeli tetap satu refund 75%; tiket lainnya 100%
	if n, persen, _, _ := refundTiket(t, db, b.tiketID[0]); n != 1 || persen != 75 {
		t.Errorf("tiket B yang sudah dibatalkan: %d refund, %d%%; mau tetap 1 refund 75%%", n, persen)
	}
	if n, persen, _, _ := refundTiket(t, db, b.tiketID[1]); n != 1 || persen != 100 {
		t.Errorf("tiket B lainnya: %d refund, %d%%", n, persen)
	}
	// C: pesanan HELD -> CANCELLED, tanpa refund
	if s := teks(t, db, "SELECT status FROM orders WHERE id = ?", c.orderID); s != pesananCancelled {
		t.Errorf("pesanan HELD: %s, mau CANCELLED", s)
	}
	if s := teks(t, db, "SELECT status FROM tickets WHERE id = ?", c.tiketID[0]); s != tiketCancelled {
		t.Errorf("tiket HELD: %s, mau CANCELLED", s)
	}
	// D dan E tidak diubah, tanpa refund
	if s := teks(t, db, "SELECT status FROM orders WHERE id = ?", d.orderID); s != "EXPIRED" {
		t.Errorf("pesanan EXPIRED berubah menjadi %s", s)
	}
	if s := teks(t, db, "SELECT status FROM tickets WHERE id = ?", ee.tiketID[0]); s != "USED" {
		t.Errorf("tiket USED berubah menjadi %s", s)
	}
	for _, id := range []int64{c.tiketID[0], d.tiketID[0], ee.tiketID[0]} {
		if n, _, _, _ := refundTiket(t, db, id); n != 0 {
			t.Errorf("tiket %d mendapat refund padahal tidak ACTIVE", id)
		}
	}

	// event CANCELLED, penjualan tertutup (tidak tampil di katalog)
	if s := teks(t, db, "SELECT status FROM events WHERE id = ?", e.eventID); s != event.StatusCancelled {
		t.Errorf("status event %s, mau CANCELLED", s)
	}
	dibuka, _ := event.AmbilEventDibuka(ctx, db, time.Now())
	if len(dibuka) != 0 {
		t.Errorf("event yang dibatalkan masih tampil di katalog")
	}
	// kuota dan promo tidak diubah
	if got := bacaTahap(t, db, e.tahapID[0]); got != sisaSebelum {
		t.Errorf("kuota berubah: %+v, sebelumnya %+v", got, sisaSebelum)
	}
	if n := hitung(t, db, "SELECT terpakai FROM promo_codes WHERE id = ?", promoID); n != 2 {
		t.Errorf("terpakai promo %d, mau tetap 2", n)
	}

	// setelah event batal, pembeli tidak bisa membatalkan tiket lagi
	_, err = batalkanTiket(ctx, db, ee.tiketID[0], u4, time.Now())
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "sudah dibatalkan penyelenggara") {
		t.Errorf("batal tiket setelah event batal: %v", err)
	}
}

func TestBR20HanyaEventOpenYangBelumDimulai(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()

	draft := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	db.Exec("UPDATE events SET status = 'DRAFT' WHERE id = ?", draft.eventID)
	mulai := siapkanEvent(t, db, time.Now().Add(-time.Hour), []tahapUji{{"Normal", 150000, 10, 20}})
	dibuka := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})

	var salah KesalahanInput
	if _, err := batalkanEvent(ctx, db, draft.eventID, time.Now()); !errors.As(err, &salah) {
		t.Errorf("event DRAFT: dapat %v, mau ditolak", err)
	}
	if _, err := batalkanEvent(ctx, db, mulai.eventID, time.Now()); !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "sudah dimulai") {
		t.Errorf("event sudah dimulai: dapat %v, mau ditolak", err)
	}
	if _, err := batalkanEvent(ctx, db, dibuka.eventID, time.Now()); err != nil {
		t.Fatalf("event OPEN: %v", err)
	}
	if _, err := batalkanEvent(ctx, db, dibuka.eventID, time.Now()); !errors.As(err, &salah) {
		t.Errorf("batal event dua kali: dapat %v, mau ditolak", err)
	}
	if _, err := batalkanEvent(ctx, db, 999999, time.Now()); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("event tidak ada: dapat %v", err)
	}
}

// Pembeli membatalkan tiket tepat ketika admin membatalkan event. Siapa pun yang menang,
// setiap tiket harus punya tepat satu refund:
//   - pembeli menang: tiket itu 75% dan kuotanya kembali, lalu batal event tidak me-refund lagi
//   - event menang: tiket itu 100%, dan pembeli mendapat pesan "sudah dibatalkan penyelenggara"
//
// Diulang 10 putaran.
func TestBR20BatalTiketBersamaanDenganBatalEvent(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	menangPembeli, menangEvent := 0, 0

	for putaran := 1; putaran <= 10; putaran++ {
		e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 100000, 10, 20}})
		userID := buatPembeli(t, db)
		p := buatPesanan(t, db, userID, e, "PAID", 2, 0, 0, tanpaPromo())

		var wg sync.WaitGroup
		var errPembeli, errEvent error
		mulai := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-mulai
			_, errPembeli = batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now())
		}()
		go func() {
			defer wg.Done()
			<-mulai
			// putaran genap: admin sedikit terlambat, supaya cabang "pembeli menang" juga teruji
			if putaran%2 == 0 {
				time.Sleep(50 * time.Millisecond)
			}
			_, errEvent = batalkanEvent(ctx, db, e.eventID, time.Now())
		}()
		close(mulai)
		wg.Wait()

		if errEvent != nil {
			t.Fatalf("putaran %d: batal event gagal: %v", putaran, errEvent)
		}

		n0, persen0, _, _ := refundTiket(t, db, p.tiketID[0])
		n1, persen1, _, _ := refundTiket(t, db, p.tiketID[1])
		if n0 != 1 || n1 != 1 || persen1 != 100 {
			t.Fatalf("putaran %d: refund tiket 1 = %d baris, tiket 2 = %d baris (%d%%); mau masing-masing 1", putaran, n0, n1, persen1)
		}
		sisa := bacaTahap(t, db, e.tahapID[0]).sisa

		switch {
		case errPembeli == nil:
			// pembeli lebih dulu: 75%, kuota kembali (10 - 2 + 1)
			menangPembeli++
			if persen0 != 75 || sisa != 9 {
				t.Errorf("putaran %d (pembeli menang): tiket 1 %d%%, sisa %d; mau 75%% dan 9", putaran, persen0, sisa)
			}
		default:
			// event lebih dulu: 100%, kuota tidak berubah, pembeli mendapat pesan yang jelas
			menangEvent++
			var salah KesalahanInput
			if !errors.As(errPembeli, &salah) || !strings.Contains(salah.Pesan, "sudah dibatalkan penyelenggara") {
				t.Errorf("putaran %d (event menang): pembeli mendapat %v", putaran, errPembeli)
			}
			if persen0 != 100 || sisa != 8 {
				t.Errorf("putaran %d (event menang): tiket 1 %d%%, sisa %d; mau 100%% dan 8", putaran, persen0, sisa)
			}
		}
	}
	t.Logf("pembeli menang %d kali, event menang %d kali", menangPembeli, menangEvent)
	if menangPembeli == 0 || menangEvent == 0 {
		t.Errorf("kedua kemungkinan urutan harus teruji: pembeli menang %d, event menang %d", menangPembeli, menangEvent)
	}
}
