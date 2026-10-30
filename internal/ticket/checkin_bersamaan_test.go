package ticket

// Test check-in (paket checkin) yang berjalan bersamaan dengan pembatalan tiket, pembatalan event,
// dan pemindahan tiket. Ditulis di paket ticket karena fungsi-fungsi itu tidak diekspor,
// sedangkan checkin.CheckIn diekspor.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/checkin"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// hariEventUji: event 30 hari lagi pukul 19:00 WIB; pintu dibuka pukul 17:00 di hari yang sama
func hariEventUji() (tanggalEvent, bukaPintu time.Time) {
	n := time.Now().In(web.Jakarta).AddDate(0, 0, 30)
	tanggalEvent = time.Date(n.Year(), n.Month(), n.Day(), 19, 0, 0, 0, web.Jakarta)
	return tanggalEvent, tanggalEvent.Add(-2 * time.Hour)
}

// jalankanBersamaan menjalankan a dan b pada saat yang sama. Jika tundaB, b dimulai 50 ms kemudian
// supaya urutan "a lebih dulu" juga teruji.
func jalankanBersamaan(a, b func(), tundaB bool) {
	var wg sync.WaitGroup
	mulai := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-mulai
		a()
	}()
	go func() {
		defer wg.Done()
		<-mulai
		if tundaB {
			time.Sleep(50 * time.Millisecond)
		}
		b()
	}()
	close(mulai)
	wg.Wait()
}

// Check-in bersamaan dengan pembatalan tiket. Dalam kenyataan keduanya tidak pernah terjadi bersamaan
// (batal hanya sampai H-7, check-in hanya di hari event), jadi waktu "sekarang" untuk pembatalan
// sengaja dibuat H-10 supaya jaminan kuncinya bisa diuji. Hasilnya harus tepat salah satu.
func TestCheckInBersamaanDenganBatalTiket(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	tanggal, pintu := hariEventUji()
	petugas := buatPembeli(t, db)
	menangMasuk, menangBatal := 0, 0

	for putaran := 1; putaran <= 10; putaran++ {
		e := siapkanEvent(t, db, tanggal, []tahapUji{{"Normal", 100000, 10, 20}})
		pemilik := buatPembeli(t, db)
		p := buatPesanan(t, db, pemilik, e, "PAID", 1, 0, 0, tanpaPromo())
		kode := teks(t, db, "SELECT kode_tiket FROM tickets WHERE id = ?", p.tiketID[0])

		var errMasuk, errBatal error
		jalankanBersamaan(
			func() { _, errBatal = batalkanTiket(ctx, db, p.tiketID[0], pemilik, tanggal.AddDate(0, 0, -10)) },
			func() { _, errMasuk = checkin.CheckIn(ctx, db, e.eventID, kode, petugas, pintu) },
			putaran%2 == 0,
		)

		masuk := hitung(t, db, "SELECT COUNT(*) FROM checkins WHERE ticket_id = ?", p.tiketID[0])
		refund := hitung(t, db, "SELECT COUNT(*) FROM refunds WHERE ticket_id = ?", p.tiketID[0])
		if masuk+refund != 1 {
			t.Fatalf("putaran %d: checkins %d + refund %d, mau tepat salah satu", putaran, masuk, refund)
		}
		var salah checkin.KesalahanInput
		var salahBatal KesalahanInput
		if masuk == 1 {
			menangMasuk++
			if errMasuk != nil || !errors.As(errBatal, &salahBatal) {
				t.Errorf("putaran %d (masuk menang): errMasuk %v, errBatal %v", putaran, errMasuk, errBatal)
			}
		} else {
			menangBatal++
			if errBatal != nil || !errors.As(errMasuk, &salah) || !strings.Contains(salah.Pesan, "dananya dikembalikan") {
				t.Errorf("putaran %d (batal menang): errBatal %v, errMasuk %v", putaran, errBatal, errMasuk)
			}
		}
	}
	t.Logf("masuk menang %d kali, batal menang %d kali", menangMasuk, menangBatal)
	if menangMasuk == 0 || menangBatal == 0 {
		t.Errorf("kedua urutan harus teruji: masuk %d, batal %d", menangMasuk, menangBatal)
	}
}

// Situasi nyata: pintu sudah dibuka pukul 17:00, admin membatalkan event pukul 17:00 juga.
// Setiap tiket harus berakhir tepat salah satu: sudah masuk (tanpa refund), atau di-refund 100% (check-in ditolak).
func TestCheckInBersamaanDenganBatalEvent(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	tanggal, pintu := hariEventUji()
	petugas := buatPembeli(t, db)
	menangMasuk, menangBatal := 0, 0

	for putaran := 1; putaran <= 10; putaran++ {
		e := siapkanEvent(t, db, tanggal, []tahapUji{{"Normal", 100000, 10, 20}})
		pemilik := buatPembeli(t, db)
		p := buatPesanan(t, db, pemilik, e, "PAID", 2, 0, 0, tanpaPromo())
		kode := teks(t, db, "SELECT kode_tiket FROM tickets WHERE id = ?", p.tiketID[0])

		var errMasuk, errEvent error
		jalankanBersamaan(
			func() { _, errEvent = batalkanEvent(ctx, db, e.eventID, pintu) },
			func() { _, errMasuk = checkin.CheckIn(ctx, db, e.eventID, kode, petugas, pintu) },
			putaran%2 == 0,
		)
		if errEvent != nil {
			t.Fatalf("putaran %d: batal event gagal: %v", putaran, errEvent)
		}

		masuk := hitung(t, db, "SELECT COUNT(*) FROM checkins WHERE ticket_id = ?", p.tiketID[0])
		refund := hitung(t, db, "SELECT COUNT(*) FROM refunds WHERE ticket_id = ?", p.tiketID[0])
		if masuk+refund != 1 {
			t.Fatalf("putaran %d: checkins %d + refund %d, mau tepat salah satu", putaran, masuk, refund)
		}
		// tiket kedua (tidak dipindai) selalu di-refund 100%
		if n, persen, _, _ := refundTiket(t, db, p.tiketID[1]); n != 1 || persen != 100 {
			t.Errorf("putaran %d: tiket kedua %d refund %d%%", putaran, n, persen)
		}
		if masuk == 1 {
			menangMasuk++
			if errMasuk != nil || teks(t, db, "SELECT status FROM tickets WHERE id = ?", p.tiketID[0]) != "USED" {
				t.Errorf("putaran %d (masuk menang): errMasuk %v", putaran, errMasuk)
			}
		} else {
			menangBatal++
			var salah checkin.KesalahanInput
			if !errors.As(errMasuk, &salah) || !strings.Contains(salah.Pesan, "dananya dikembalikan") {
				t.Errorf("putaran %d (batal menang): errMasuk %v", putaran, errMasuk)
			}
		}
	}
	t.Logf("masuk menang %d kali, batal event menang %d kali", menangMasuk, menangBatal)
	if menangMasuk == 0 || menangBatal == 0 {
		t.Errorf("kedua urutan harus teruji: masuk %d, batal event %d", menangMasuk, menangBatal)
	}
}

// Situasi nyata: pemegang lama memindai kodenya di pintu tepat saat pemilik memindahkan tiket.
//   - check-in lebih dulu: tiket USED, pemindahan ditolak "sudah dipakai"
//   - pemindahan lebih dulu: kode lama ditolak dengan pesan "sudah dipindahkan ke ..." (bukan error)
func TestCheckInBersamaanDenganPindahTiket(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	tanggal, pintu := hariEventUji()
	petugas := buatPembeli(t, db)
	menangMasuk, menangPindah := 0, 0

	for putaran := 1; putaran <= 10; putaran++ {
		e := siapkanEvent(t, db, tanggal, []tahapUji{{"Normal", 100000, 10, 20}})
		pemilik := buatPembeli(t, db)
		p := buatPesanan(t, db, pemilik, e, "PAID", 1, 0, 0, tanpaPromo())
		kodeLama := teks(t, db, "SELECT kode_tiket FROM tickets WHERE id = ?", p.tiketID[0])
		penerima := fmt.Sprintf("Penerima %d", putaran)

		var errMasuk, errPindah error
		jalankanBersamaan(
			func() { _, errPindah = pindahkanTiket(ctx, db, p.tiketID[0], pemilik, penerima, pintu) },
			func() { _, errMasuk = checkin.CheckIn(ctx, db, e.eventID, kodeLama, petugas, pintu) },
			putaran%2 == 0,
		)

		masuk := hitung(t, db, "SELECT COUNT(*) FROM checkins WHERE ticket_id = ?", p.tiketID[0])
		pindah := hitung(t, db, "SELECT COUNT(*) FROM ticket_transfers WHERE ticket_id = ?", p.tiketID[0])
		if masuk+pindah != 1 {
			t.Fatalf("putaran %d: checkins %d + transfer %d, mau tepat salah satu", putaran, masuk, pindah)
		}
		var salahMasuk checkin.KesalahanInput
		var salahPindah KesalahanInput
		if masuk == 1 {
			menangMasuk++
			if errMasuk != nil || !errors.As(errPindah, &salahPindah) || !strings.Contains(salahPindah.Pesan, "sudah dipakai") {
				t.Errorf("putaran %d (masuk menang): errMasuk %v, errPindah %v", putaran, errMasuk, errPindah)
			}
		} else {
			menangPindah++
			if errPindah != nil || !errors.As(errMasuk, &salahMasuk) || !strings.Contains(salahMasuk.Pesan, "sudah dipindahkan ke "+penerima) {
				t.Errorf("putaran %d (pindah menang): errPindah %v, errMasuk %v", putaran, errPindah, errMasuk)
			}
		}
	}
	t.Logf("masuk menang %d kali, pindah menang %d kali", menangMasuk, menangPindah)
	if menangMasuk == 0 || menangPindah == 0 {
		t.Errorf("kedua urutan harus teruji: masuk %d, pindah %d", menangMasuk, menangPindah)
	}
}
