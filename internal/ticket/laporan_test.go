package ticket

// Laporan (paket report) setelah pembatalan sungguhan oleh pembeli dan oleh penyelenggara.
// Ditulis di paket ticket karena batalkanTiket dan batalkanEvent tidak diekspor.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/report"
)

// eventPromo: Festival Rp150.000, dua pesanan PAID (u1: 2 tiket berpromo 15%, u2: 1 tiket tanpa promo)
func eventPromo(t *testing.T, db *sql.DB) (e eventUji, u1 int64, a, b pesananUji) {
	t.Helper()
	e = siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 20, 20}})
	res, _ := db.Exec("INSERT INTO promo_codes (kode, persen, kuota_pemakaian, terpakai) VALUES ('HEMAT15', 15, 5, 1)")
	promoID, _ := res.LastInsertId()
	u1 = buatPembeli(t, db)
	a = buatPesanan(t, db, u1, e, "PAID", 2, 0, 22500, sql.NullInt64{Int64: promoID, Valid: true})
	b = buatPesanan(t, db, buatPembeli(t, db), e, "PAID", 1, 0, 0, tanpaPromo())
	return
}

func laporan(t *testing.T, db *sql.DB, eventID int64) report.Laporan {
	t.Helper()
	lap, err := report.AmbilLaporan(context.Background(), db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	return lap
}

// Pembatalan oleh pembeli: refund 75% dari harga bayar, 25% tetap menjadi pendapatan
func TestLaporanSetelahPembatalanPembeliMenyisakan25Persen(t *testing.T) {
	db := bukaDBKosong(t)
	e, u1, a, _ := eventPromo(t, db)

	if _, err := batalkanTiket(context.Background(), db, a.tiketID[0], u1, time.Now()); err != nil {
		t.Fatal(err)
	}

	total := laporan(t, db, e.eventID).Total
	// kotor 3 x 150.000; potongan 2 x 22.500; refund 75% x 127.500 = 95.625
	mau := report.Angka{Terjual: 3, TiketRefund: 1, Kotor: 450000, Potongan: 45000, Refund: 95625}
	if total != mau {
		t.Errorf("total %+v, mau %+v", total, mau)
	}
	// tiket yang dibatalkan menyisakan 127.500 - 95.625 = 31.875 (25%); dua tiket lain 127.500 + 150.000
	if got := total.Bersih(); got != 31875+127500+150000 {
		t.Errorf("bersih %d, mau %d", got, 31875+127500+150000)
	}
}

// Keputusan Opsi A (PRD §9), kasus 1: event dibatalkan tanpa pembatalan pembeli sebelumnya -> bersih Rp0
func TestLaporanEventDibatalkanTanpaPembatalanPembeliBersihNol(t *testing.T) {
	db := bukaDBKosong(t)
	e, _, _, _ := eventPromo(t, db)
	// pesanan HELD ikut dibatalkan dan tidak boleh mempengaruhi laporan
	buatPesanan(t, db, buatPembeli(t, db), e, "HELD", 1, 0, 0, tanpaPromo())

	if _, err := batalkanEvent(context.Background(), db, e.eventID, time.Now()); err != nil {
		t.Fatal(err)
	}

	lap := laporan(t, db, e.eventID)
	if lap.StatusEvent != "CANCELLED" {
		t.Errorf("status %s", lap.StatusEvent)
	}
	// semua tiket terjual di-refund 100% dari harga bayar: 2 x 127.500 + 150.000
	mau := report.Angka{Terjual: 3, TiketRefund: 3, Kotor: 450000, Potongan: 45000, Refund: 405000}
	if lap.Total != mau || lap.Total.Bersih() != 0 {
		t.Errorf("total %+v (bersih %d), mau %+v (bersih 0)", lap.Total, lap.Total.Bersih(), mau)
	}
}

// Keputusan Opsi A (PRD §9), kasus 2: satu pembeli sudah membatalkan (75%) sebelum event dibatalkan.
// Tiket itu tidak di-refund lagi (sudah REFUNDED), jadi 25% harga bayarnya tetap tercatat sebagai pendapatan.
func TestLaporanEventDibatalkanSetelahPembatalanPembeliMenyisakan25Persen(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e, u1, a, _ := eventPromo(t, db)

	if _, err := batalkanTiket(ctx, db, a.tiketID[0], u1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := batalkanEvent(ctx, db, e.eventID, time.Now()); err != nil {
		t.Fatal(err)
	}

	total := laporan(t, db, e.eventID).Total
	// refund: 95.625 (75%, pembeli) + 127.500 + 150.000 (100%, event)
	mau := report.Angka{Terjual: 3, TiketRefund: 3, Kotor: 450000, Potongan: 45000, Refund: 95625 + 127500 + 150000}
	if total != mau {
		t.Errorf("total %+v, mau %+v", total, mau)
	}
	// bersih = 25% dari 127.500
	if got := total.Bersih(); got != 31875 {
		t.Errorf("bersih %d, mau 31875 (25%% dari harga bayar tiket yang dibatalkan pembeli)", got)
	}
}
