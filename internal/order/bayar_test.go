package order

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// pesanSatu membuat pesanan n tiket pada waktu `now` dan mengembalikan id-nya
func pesanSatu(t *testing.T, e eventUji, userID int64, n int, now time.Time, db *sql.DB) int64 {
	t.Helper()
	id, err := buatPesanan(context.Background(), db, userID, e.eventID, pesan(e.kategoriID, n), now)
	if err != nil {
		t.Fatalf("buat pesanan: %v", err)
	}
	return id
}

func statusPesanan(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var s string
	if err := db.QueryRow("SELECT status FROM orders WHERE id = ?", id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// ===== BR-11: pembayaran =====

func TestBR11BayarTepatWaktuMemberiKodeUnik(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()
	orderID := pesanSatu(t, e, userID, 3, now, db)

	if err := bayarPesanan(ctx, db, orderID, userID, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("bayar: %v", err)
	}

	p, _ := ambilPesananMilik(ctx, db, orderID, userID)
	if p.Status != StatusPaid || !p.PaidAt.Valid {
		t.Errorf("status %s, paid_at %v; mau PAID dengan waktu bayar", p.Status, p.PaidAt)
	}
	kode := map[string]bool{}
	for _, tk := range p.Tiket {
		if tk.Status != TiketActive || !strings.HasPrefix(tk.KodeTiket, "TKT-") {
			t.Errorf("tiket %+v, mau ACTIVE dengan kode TKT-", tk)
		}
		kode[tk.KodeTiket] = true
	}
	if len(kode) != 3 {
		t.Errorf("kode tiket tidak unik: %v", kode)
	}
	// kuota tetap berkurang (tidak dikembalikan)
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 7 {
		t.Errorf("sisa %d, mau 7", got.sisa)
	}
}

func TestBR11BayarTepatDiDetikBatas(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()
	orderID := pesanSatu(t, e, userID, 1, now, db)

	if err := bayarPesanan(context.Background(), db, orderID, userID, now.Add(15*time.Minute)); err != nil {
		t.Errorf("bayar tepat di batas 15 menit ditolak: %v", err)
	}
}

func TestBR10BayarTerlambatMembuatExpired(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()
	orderID := pesanSatu(t, e, userID, 2, now, db)

	err := bayarPesanan(context.Background(), db, orderID, userID, now.Add(15*time.Minute+time.Second))
	if !errors.Is(err, ErrWaktuBayarHabis) {
		t.Fatalf("mau ErrWaktuBayarHabis, dapat %v", err)
	}
	if s := statusPesanan(t, db, orderID); s != StatusExpired {
		t.Errorf("status %s, mau EXPIRED", s)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM tickets WHERE order_id = ? AND status = 'CANCELLED'", orderID); n != 2 {
		t.Errorf("tiket CANCELLED %d, mau 2", n)
	}
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 10 {
		t.Errorf("sisa %d, mau kembali 10", got.sisa)
	}
}

func TestBayarDitolakJikaSudahDibayarAtauBukanPemilik(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	orangLain := buatPembeli(t, db)
	now := waktuSekarang()
	orderID := pesanSatu(t, e, userID, 1, now, db)

	if err := bayarPesanan(ctx, db, orderID, orangLain, now); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("orang lain membayar: dapat %v, mau ErrTidakDitemukan", err)
	}
	if err := bayarPesanan(ctx, db, orderID, userID, now); err != nil {
		t.Fatal(err)
	}
	var salah KesalahanInput
	if err := bayarPesanan(ctx, db, orderID, userID, now); !errors.As(err, &salah) {
		t.Errorf("bayar dua kali: dapat %v, mau KesalahanInput", err)
	}
	if err := bayarPesanan(ctx, db, 999999, userID, now); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("pesanan tidak ada: dapat %v", err)
	}
}

// ===== BR-10: worker kedaluwarsa =====

func TestBR10WorkerHanyaMemprosesPesananYangLewatWaktu(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()

	lama := pesanSatu(t, e, userID, 2, now.Add(-20*time.Minute), db) // batas bayar sudah lewat 5 menit
	baru := pesanSatu(t, e, userID, 3, now.Add(-5*time.Minute), db)  // masih 10 menit lagi
	lunas := pesanSatu(t, e, userID, 1, now.Add(-30*time.Minute), db)
	if err := bayarPesanan(context.Background(), db, lunas, userID, now.Add(-29*time.Minute)); err != nil {
		t.Fatal(err)
	}

	jumlah, err := ExpireKedaluwarsa(context.Background(), db, now)
	if err != nil {
		t.Fatal(err)
	}
	if jumlah != 1 {
		t.Errorf("di-expire %d pesanan, mau 1", jumlah)
	}
	if statusPesanan(t, db, lama) != StatusExpired || statusPesanan(t, db, baru) != StatusHeld || statusPesanan(t, db, lunas) != StatusPaid {
		t.Error("status setelah worker tidak sesuai")
	}
	// 10 - 2 - 3 - 1 + 2 (kembali dari pesanan lama) = 6
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 6 {
		t.Errorf("sisa %d, mau 6", got.sisa)
	}

	// putaran kedua tidak memproses ulang pesanan yang sudah EXPIRED
	if jumlah, _ := ExpireKedaluwarsa(context.Background(), db, now); jumlah != 0 {
		t.Errorf("putaran kedua memproses %d pesanan, mau 0", jumlah)
	}
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 6 {
		t.Errorf("kuota dikembalikan dua kali: sisa %d", got.sisa)
	}
}

// Opsi B di alur nyata: pesanan diambil dari Presale, lalu kedaluwarsa setelah Presale berakhir
func TestBR10KuotaPindahKeTahapBerikutnyaJikaTahapAsalLewat(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Presale", 100000, 5, 1}, {"Normal", 150000, 5, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()
	pesanSatu(t, e, userID, 2, now, db) // 2 tiket Presale

	// worker baru memproses 2 hari kemudian, saat Presale sudah berakhir
	if _, err := ExpireKedaluwarsa(context.Background(), db, now.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if got := bacaTahap(t, db, e.tahapID[0]); got != (kuotaSisa{3, 3}) {
		t.Errorf("Presale: %+v, mau kuota 3 sisa 3", got)
	}
	if got := bacaTahap(t, db, e.tahapID[1]); got != (kuotaSisa{7, 7}) {
		t.Errorf("Normal: %+v, mau kuota 7 sisa 7", got)
	}
	if n := hitung(t, db, "SELECT SUM(kuota) FROM price_tiers WHERE category_id = ?", e.kategoriID); n != 10 {
		t.Errorf("BR-04 rusak: total kuota tahap %d, mau 10", n)
	}
}

// Pembeli menekan "Bayar" tepat ketika worker sedang berjalan. Apa pun yang menang,
// setiap pesanan harus berakhir PAID atau EXPIRED, dan kuota hanya kembali sekali.
func TestBR10BayarDanWorkerBersamaanTidakMengembalikanKuotaDuaKali(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 20, 20}})
	userID := buatPembeli(t, db)
	now := waktuSekarang()

	// 10 pesanan @1 tiket, semuanya tepat di batas bayar pada saat `now`
	var pesanan []int64
	for i := 0; i < 10; i++ {
		pesanan = append(pesanan, pesanSatu(t, e, userID, 1, now.Add(-15*time.Minute), db))
	}

	var wg sync.WaitGroup
	mulai := make(chan struct{})
	// pembeli: masih tepat waktu (detik batas)
	for _, id := range pesanan {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-mulai
			err := bayarPesanan(context.Background(), db, id, userID, now)
			var salah KesalahanInput
			if err != nil && !errors.As(err, &salah) {
				t.Errorf("bayar: %v", err)
			}
		}(id)
	}
	// worker: waktunya 1 detik lebih lambat, jadi menganggap semua pesanan kedaluwarsa
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-mulai
		if _, err := ExpireKedaluwarsa(context.Background(), db, now.Add(time.Second)); err != nil {
			t.Errorf("worker: %v", err)
		}
	}()
	close(mulai)
	wg.Wait()

	dibayar := hitung(t, db, "SELECT COUNT(*) FROM orders WHERE status = 'PAID'")
	kedaluwarsa := hitung(t, db, "SELECT COUNT(*) FROM orders WHERE status = 'EXPIRED'")
	if dibayar+kedaluwarsa != 10 {
		t.Errorf("PAID %d + EXPIRED %d, mau total 10", dibayar, kedaluwarsa)
	}
	// sisa kuota = kuota - tiket yang dibayar; kuota pesanan EXPIRED kembali tepat sekali
	if got := bacaTahap(t, db, e.tahapID[0]); got.sisa != 20-dibayar {
		t.Errorf("sisa %d, mau %d (PAID %d, EXPIRED %d)", got.sisa, 20-dibayar, dibayar, kedaluwarsa)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM tickets WHERE status = 'ACTIVE'"); n != dibayar {
		t.Errorf("tiket ACTIVE %d, mau %d", n, dibayar)
	}
}
