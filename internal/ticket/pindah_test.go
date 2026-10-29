package ticket

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBR21BR22PindahBerhasilDanKodeLamaTidakBerlaku(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	p := buatPesanan(t, db, userID, e, "PAID", 2, 0, 0, tanpaPromo())
	kodeAwal := teks(t, db, "SELECT kode_tiket FROM tickets WHERE id = ?", p.tiketID[0])
	sisaSebelum := bacaTahap(t, db, e.tahapID[0])

	hasil, err := pindahkanTiket(ctx, db, p.tiketID[0], userID, "  Ani Baru  ", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	// BR-22: kode baru berbeda dan berformat benar; kode lama tercatat
	if hasil.KodeLama != kodeAwal || hasil.KodeBaru == kodeAwal || !strings.HasPrefix(hasil.KodeBaru, "TKT-") || len(hasil.KodeBaru) != 12 {
		t.Errorf("hasil %+v, kode awal %s", hasil, kodeAwal)
	}
	var nama, kode string
	var sudah bool
	db.QueryRow("SELECT nama_pemegang, kode_tiket, sudah_dipindah FROM tickets WHERE id = ?", p.tiketID[0]).Scan(&nama, &kode, &sudah)
	if nama != "Ani Baru" || kode != hasil.KodeBaru || !sudah {
		t.Errorf("tiket: nama %q, kode %s, sudah_dipindah %v", nama, kode, sudah)
	}
	// kode lama tidak ada lagi di tiket mana pun, tetapi tersimpan di ticket_transfers
	if n := hitung(t, db, "SELECT COUNT(*) FROM tickets WHERE kode_tiket = ?", kodeAwal); n != 0 {
		t.Errorf("kode lama masih dipakai %d tiket", n)
	}
	var namaLama, namaBaru string
	var pemindah int64
	db.QueryRow("SELECT nama_lama, nama_baru, user_id FROM ticket_transfers WHERE kode_lama = ?", kodeAwal).Scan(&namaLama, &namaBaru, &pemindah)
	if namaLama != "Pemegang 1" || namaBaru != "Ani Baru" || pemindah != userID {
		t.Errorf("transfer tersimpan: %q -> %q oleh %d", namaLama, namaBaru, pemindah)
	}
	// tiket lain di pesanan dan kuota tidak berubah
	if s := teks(t, db, "SELECT nama_pemegang FROM tickets WHERE id = ?", p.tiketID[1]); s != "Pemegang 2" {
		t.Errorf("tiket lain ikut berubah: %s", s)
	}
	if got := bacaTahap(t, db, e.tahapID[0]); got != sisaSebelum {
		t.Errorf("kuota berubah: %+v", got)
	}

	// BR-21: pemindahan kedua ditolak
	_, err = pindahkanTiket(ctx, db, p.tiketID[0], userID, "Cici", time.Now())
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "satu kali") {
		t.Errorf("pindah kedua: dapat %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM ticket_transfers"); n != 1 {
		t.Errorf("jumlah transfer %d, mau 1", n)
	}
}

func TestBR23PindahDitolakTanpaPerubahan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 20, 20}})
	userID := buatPembeli(t, db)
	orangLain := buatPembeli(t, db)

	dipakai := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())
	db.Exec("UPDATE tickets SET status = 'USED' WHERE id = ?", dipakai.tiketID[0])
	direfund := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())
	if _, err := batalkanTiket(ctx, db, direfund.tiketID[0], userID, time.Now()); err != nil {
		t.Fatal(err)
	}
	ditahan := buatPesanan(t, db, userID, e, "HELD", 1, 0, 0, tanpaPromo())
	lunas := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())

	eventBatal := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 5, 20}})
	tiketEventBatal := buatPesanan(t, db, userID, eventBatal, "PAID", 1, 0, 0, tanpaPromo())
	db.Exec("UPDATE events SET status = 'CANCELLED' WHERE id = ?", eventBatal.eventID)

	eventMulai := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 5, 20}})
	tiketEventMulai := buatPesanan(t, db, userID, eventMulai, "PAID", 1, 0, 0, tanpaPromo())
	db.Exec("UPDATE events SET tanggal_event = ? WHERE id = ?", time.Now().Add(-time.Minute), eventMulai.eventID)

	kasus := []struct {
		nama  string
		tiket int64
		pesan string
	}{
		{"tiket USED", dipakai.tiketID[0], "sudah dipakai"},
		{"tiket REFUNDED", direfund.tiketID[0], "tidak aktif"},
		{"tiket HELD", ditahan.tiketID[0], "tidak aktif"},
		{"event dibatalkan", tiketEventBatal.tiketID[0], "tidak sedang dibuka"},
		{"event sudah dimulai", tiketEventMulai.tiketID[0], "sudah dimulai"},
		{"nama baru kosong", lunas.tiketID[0], "wajib diisi"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			nama := "Nama Baru"
			if k.nama == "nama baru kosong" {
				nama = "  "
			}
			_, err := pindahkanTiket(ctx, db, k.tiket, userID, nama, time.Now())
			var salah KesalahanInput
			if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, k.pesan) {
				t.Errorf("mau ditolak berisi %q, dapat %v", k.pesan, err)
			}
		})
	}

	// bukan pemilik pesanan: dianggap tidak ada
	if _, err := pindahkanTiket(ctx, db, lunas.tiketID[0], orangLain, "Nama Baru", time.Now()); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("bukan pemilik: dapat %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM ticket_transfers"); n != 0 {
		t.Errorf("ada %d transfer padahal semua ditolak", n)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM tickets WHERE sudah_dipindah = 1 OR nama_pemegang = 'Nama Baru'"); n != 0 {
		t.Errorf("ada %d tiket berubah padahal semua ditolak", n)
	}
}

// Keputusan di PRD §9: tiket yang sudah dipindahkan tidak bisa dibatalkan pembeli,
// tetapi tetap di-refund 100% ke pemilik pesanan jika event dibatalkan
func TestTiketDipindahTidakBisaDibatalkanTapiTetapDirefundSaatEventBatal(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	userID := buatPembeli(t, db)
	p := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())
	if _, err := pindahkanTiket(ctx, db, p.tiketID[0], userID, "Ani", time.Now()); err != nil {
		t.Fatal(err)
	}

	_, err := batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now())
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "sudah dipindahkan") {
		t.Fatalf("batal tiket yang dipindah: dapat %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM refunds"); n != 0 {
		t.Errorf("ada %d refund padahal pembatalan ditolak", n)
	}

	if _, err := batalkanEvent(ctx, db, e.eventID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n, persen, nominal, penerima := refundTiket(t, db, p.tiketID[0]); n != 1 || persen != 100 || nominal != 150000 || penerima != userID {
		t.Errorf("refund saat event batal: %d baris, %d%%, Rp%d, ke user %d", n, persen, nominal, penerima)
	}
}

// Pemilik menekan "Pindahkan" dua kali bersamaan dengan nama berbeda (mis. dua tab browser).
// Diulang 10 putaran.
func TestBR21DuaPemindahanBersamaan(t *testing.T) {
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
		for _, nama := range []string{"Ani", "Budi"} {
			wg.Add(1)
			go func(nama string) {
				defer wg.Done()
				<-mulai
				_, err := pindahkanTiket(ctx, db, p.tiketID[0], userID, nama, time.Now())
				mu.Lock()
				defer mu.Unlock()
				var salah KesalahanInput
				switch {
				case err == nil:
					berhasil++
				case errors.As(err, &salah) && strings.Contains(salah.Pesan, "satu kali"):
					ditolak++
				default:
					t.Errorf("putaran %d: error tak terduga %v", putaran, err)
				}
			}(nama)
		}
		close(mulai)
		wg.Wait()

		if berhasil != 1 || ditolak != 1 {
			t.Fatalf("putaran %d: berhasil %d, ditolak %d; mau 1 dan 1", putaran, berhasil, ditolak)
		}
		// isi tiket sama persis dengan satu-satunya catatan pemindahan
		var namaTiket, kodeTiket, namaTransfer, kodeTransfer string
		db.QueryRow("SELECT nama_pemegang, kode_tiket FROM tickets WHERE id = ?", p.tiketID[0]).Scan(&namaTiket, &kodeTiket)
		db.QueryRow("SELECT nama_baru, kode_baru FROM ticket_transfers WHERE ticket_id = ?", p.tiketID[0]).Scan(&namaTransfer, &kodeTransfer)
		if n := hitung(t, db, "SELECT COUNT(*) FROM ticket_transfers WHERE ticket_id = ?", p.tiketID[0]); n != 1 {
			t.Errorf("putaran %d: %d catatan pemindahan, mau 1", putaran, n)
		}
		if namaTiket != namaTransfer || kodeTiket != kodeTransfer {
			t.Errorf("putaran %d: tiket %s/%s tidak sama dengan transfer %s/%s", putaran, namaTiket, kodeTiket, namaTransfer, kodeTransfer)
		}
	}
}

// Pemilik memindahkan tiket tepat ketika (di tab lain) membatalkannya.
// Tidak boleh berakhir dengan tiket yang dipindahkan sekaligus di-refund.
// Putaran ganjil: keduanya mulai bersamaan. Putaran genap: pemindahan sedikit terlambat,
// supaya urutan "batal lebih dulu" juga teruji.
func TestPindahBersamaanDenganBatal(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	menangPindah, menangBatal := 0, 0

	for putaran := 1; putaran <= 10; putaran++ {
		e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
		userID := buatPembeli(t, db)
		p := buatPesanan(t, db, userID, e, "PAID", 1, 0, 0, tanpaPromo())

		var wg sync.WaitGroup
		var errPindah, errBatal error
		mulai := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-mulai
			if putaran%2 == 0 {
				time.Sleep(50 * time.Millisecond)
			}
			_, errPindah = pindahkanTiket(ctx, db, p.tiketID[0], userID, fmt.Sprintf("Penerima %d", putaran), time.Now())
		}()
		go func() {
			defer wg.Done()
			<-mulai
			_, errBatal = batalkanTiket(ctx, db, p.tiketID[0], userID, time.Now())
		}()
		close(mulai)
		wg.Wait()

		transfer := hitung(t, db, "SELECT COUNT(*) FROM ticket_transfers WHERE ticket_id = ?", p.tiketID[0])
		refund := hitung(t, db, "SELECT COUNT(*) FROM refunds WHERE ticket_id = ?", p.tiketID[0])
		status := teks(t, db, "SELECT status FROM tickets WHERE id = ?", p.tiketID[0])
		sisa := bacaTahap(t, db, e.tahapID[0]).sisa

		if transfer+refund != 1 {
			t.Fatalf("putaran %d: transfer %d + refund %d, mau tepat salah satu", putaran, transfer, refund)
		}
		var salah KesalahanInput
		switch {
		case transfer == 1:
			// pindah lebih dulu: batal ditolak, tiket tetap ACTIVE, kuota tidak kembali
			menangPindah++
			if errPindah != nil || !errors.As(errBatal, &salah) || !strings.Contains(salah.Pesan, "sudah dipindahkan") {
				t.Errorf("putaran %d (pindah menang): errPindah %v, errBatal %v", putaran, errPindah, errBatal)
			}
			if status != tiketActive || sisa != 9 {
				t.Errorf("putaran %d (pindah menang): status %s, sisa %d; mau ACTIVE dan 9", putaran, status, sisa)
			}
		default:
			// batal lebih dulu: pindah ditolak, tiket REFUNDED, kuota kembali sekali
			menangBatal++
			if errBatal != nil || !errors.As(errPindah, &salah) || !strings.Contains(salah.Pesan, "tidak aktif") {
				t.Errorf("putaran %d (batal menang): errBatal %v, errPindah %v", putaran, errBatal, errPindah)
			}
			if status != tiketRefunded || sisa != 10 {
				t.Errorf("putaran %d (batal menang): status %s, sisa %d; mau REFUNDED dan 10", putaran, status, sisa)
			}
		}
	}
	t.Logf("pindah menang %d kali, batal menang %d kali", menangPindah, menangBatal)
	if menangPindah == 0 || menangBatal == 0 {
		t.Errorf("kedua kemungkinan urutan harus teruji: pindah menang %d, batal menang %d", menangPindah, menangBatal)
	}
}
