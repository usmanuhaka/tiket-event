package checkin

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBR24BR25BR26CheckInBerhasilLaluDitolakKeduaKali(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID, tiket := siapkanTiket(t, db, "Konser", tanggalEvent, 1)
	petugas := buatUser(t, db, "PETUGAS")

	// BR-24: periksa menampilkan nama, kategori, status (kode diketik huruf kecil dan berspasi)
	info, err := periksaKode(ctx, db, " "+strings.ToLower(tiket[0].kode)+" ")
	if err != nil {
		t.Fatal(err)
	}
	if info.NamaPemegang != "Pemegang 1" || info.NamaKategori != "VIP" || info.Status != "ACTIVE" {
		t.Errorf("info %+v", info)
	}

	// BR-25: ACTIVE, event sesuai, hari event -> masuk
	jam := diPintu(18, 42)
	hasil, err := CheckIn(ctx, db, eventID, tiket[0].kode, petugas, jam)
	if err != nil {
		t.Fatal(err)
	}
	if hasil.Status != "USED" || statusTiket(t, db, tiket[0].id) != "USED" {
		t.Errorf("status setelah check-in: %s / %s", hasil.Status, statusTiket(t, db, tiket[0].id))
	}
	var petugasTercatat int64
	var waktu time.Time
	db.QueryRow("SELECT petugas_id, waktu_masuk FROM checkins WHERE ticket_id = ?", tiket[0].id).Scan(&petugasTercatat, &waktu)
	if petugasTercatat != petugas || !waktu.Equal(jam) {
		t.Errorf("checkins: petugas %d, waktu %v", petugasTercatat, waktu)
	}

	// BR-26: pemindaian kedua ditolak, beserta waktu masuknya
	_, err = CheckIn(ctx, db, eventID, tiket[0].kode, petugas, diPintu(19, 5))
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "sudah dipakai masuk pada 20 Des 2026 18:42 WIB") {
		t.Errorf("pindai kedua: %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM checkins"); n != 1 {
		t.Errorf("jumlah checkins %d, mau 1", n)
	}
}

func TestCheckInDitolakTanpaPerubahan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID, tiket := siapkanTiket(t, db, "Konser", tanggalEvent, 5)
	eventLain, _ := siapkanTiket(t, db, "Seminar", tanggalEvent, 0)
	petugas := buatUser(t, db, "PETUGAS")

	db.Exec("UPDATE tickets SET status = 'REFUNDED' WHERE id = ?", tiket[1].id)
	db.Exec("UPDATE tickets SET status = 'HELD' WHERE id = ?", tiket[2].id)
	db.Exec("UPDATE tickets SET status = 'CANCELLED' WHERE id = ?", tiket[3].id)
	// tiket[4] dipindahkan: kode lamanya diganti dan dicatat
	db.Exec("UPDATE tickets SET kode_tiket = 'TKT-BARU0001', nama_pemegang = 'Rina Penerima', sudah_dipindah = 1 WHERE id = ?", tiket[4].id)
	db.Exec(`INSERT INTO ticket_transfers (ticket_id, user_id, nama_lama, nama_baru, kode_lama, kode_baru)
		VALUES (?, ?, 'Pemegang 5', 'Rina Penerima', ?, 'TKT-BARU0001')`, tiket[4].id, petugas, tiket[4].kode)

	kasus := []struct {
		nama    string
		eventID int64
		kode    string
		now     time.Time
		pesan   string
	}{
		{"kode tidak ada", eventID, "TKT-TIDAKADA", diPintu(18, 0), "tidak ditemukan"},
		{"kode lama hasil pemindahan", eventID, tiket[4].kode, diPintu(18, 0), "sudah dipindahkan ke Rina Penerima"},
		{"event lain", eventLain, tiket[0].kode, diPintu(18, 0), "untuk event Konser"},
		{"bukan hari event", eventID, tiket[0].kode, diPintu(18, 0).AddDate(0, 0, -1), "Bukan hari event"},
		{"REFUNDED", eventID, tiket[1].kode, diPintu(18, 0), "dananya dikembalikan"},
		{"HELD", eventID, tiket[2].kode, diPintu(18, 0), "belum dibayar"},
		{"CANCELLED", eventID, tiket[3].kode, diPintu(18, 0), "dibatalkan"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := CheckIn(ctx, db, k.eventID, k.kode, petugas, k.now)
			var salah KesalahanInput
			if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, k.pesan) {
				t.Errorf("mau ditolak berisi %q, dapat %v", k.pesan, err)
			}
		})
	}

	// kode baru hasil pemindahan tetap bisa dipakai
	if _, err := CheckIn(ctx, db, eventID, "TKT-BARU0001", petugas, diPintu(18, 0)); err != nil {
		t.Errorf("kode baru ditolak: %v", err)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM checkins"); n != 1 {
		t.Errorf("jumlah checkins %d, mau 1 (hanya kode baru)", n)
	}
	if statusTiket(t, db, tiket[0].id) != "ACTIVE" {
		t.Error("tiket yang ditolak (event lain / bukan hari event) berubah status")
	}
}

func TestAmbilEventHariIni(t *testing.T) {
	db := bukaDBKosong(t)
	hariIni, _ := siapkanTiket(t, db, "Hari Ini", tanggalEvent, 0)
	siapkanTiket(t, db, "Besok", tanggalEvent.AddDate(0, 0, 1), 0)
	batal, _ := siapkanTiket(t, db, "Batal", tanggalEvent, 0)
	db.Exec("UPDATE events SET status = 'CANCELLED' WHERE id = ?", batal)

	daftar, err := ambilEventHariIni(context.Background(), db, diPintu(8, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(daftar) != 1 || daftar[0].ID != hariIni {
		t.Errorf("event hari ini: %+v", daftar)
	}
}

// Dua petugas di dua pintu memindai kode yang sama pada saat yang sama. Diulang 10 putaran.
func TestBR26DuaPetugasMemindaiTiketYangSama(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	petugas := []int64{buatUser(t, db, "PETUGAS"), buatUser(t, db, "PETUGAS")}

	for putaran := 1; putaran <= 10; putaran++ {
		eventID, tiket := siapkanTiket(t, db, "Konser", tanggalEvent, 1)

		var wg sync.WaitGroup
		var mu sync.Mutex
		berhasil, ditolak := 0, 0
		mulai := make(chan struct{})
		for _, p := range petugas {
			wg.Add(1)
			go func(petugasID int64) {
				defer wg.Done()
				<-mulai
				_, err := CheckIn(ctx, db, eventID, tiket[0].kode, petugasID, diPintu(18, 0))
				mu.Lock()
				defer mu.Unlock()
				var salah KesalahanInput
				switch {
				case err == nil:
					berhasil++
				case errors.As(err, &salah) && strings.Contains(salah.Pesan, "sudah dipakai masuk"):
					ditolak++
				default:
					t.Errorf("putaran %d: error tak terduga %v", putaran, err)
				}
			}(p)
		}
		close(mulai)
		wg.Wait()

		if berhasil != 1 || ditolak != 1 {
			t.Fatalf("putaran %d: berhasil %d, ditolak %d; mau 1 dan 1", putaran, berhasil, ditolak)
		}
		if n := hitung(t, db, "SELECT COUNT(*) FROM checkins WHERE ticket_id = ?", tiket[0].id); n != 1 {
			t.Errorf("putaran %d: %d baris checkins, mau 1", putaran, n)
		}
	}
}

// Kode lama dipindai PERSIS saat pemindahan sedang berjalan: transaksi pemindahan sudah mengganti
// kode dan memegang kunci tiket, tetapi belum COMMIT. Check-in harus menunggu, lalu menolak dengan
// pesan "sudah dipindahkan ke ...", bukan error dan bukan mengizinkan kode lama masuk.
// Transaksi pemindahan di sini ditulis langsung dengan SQL (langkah yang sama dengan
// ticket.pindahkanTiket) supaya bisa "ditahan" di tengah jalan.
func TestKodeLamaDipindaiSaatPemindahanBerlangsung(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID, tiket := siapkanTiket(t, db, "Konser", tanggalEvent, 1)
	petugas := buatUser(t, db, "PETUGAS")

	// 1. pemindahan dimulai: kunci event lalu tiket, ganti kode dan nama, catat transfer, BELUM commit
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var kunciEvent int64
	if err := tx.QueryRow("SELECT id FROM events WHERE id = ? FOR UPDATE", eventID).Scan(&kunciEvent); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := tx.QueryRow("SELECT id FROM tickets WHERE id = ? FOR UPDATE", tiket[0].id).Scan(&id); err != nil {
		t.Fatal(err)
	}
	tx.Exec("UPDATE tickets SET kode_tiket = 'TKT-BARU0001', nama_pemegang = 'Rina Penerima', sudah_dipindah = 1 WHERE id = ?", id)
	tx.Exec(`INSERT INTO ticket_transfers (ticket_id, user_id, nama_lama, nama_baru, kode_lama, kode_baru)
		VALUES (?, ?, 'Pemegang 1', 'Rina Penerima', ?, 'TKT-BARU0001')`, id, petugas, tiket[0].kode)

	// 2. petugas memindai kode lama saat itu juga
	hasil := make(chan error, 1)
	go func() {
		_, err := CheckIn(ctx, db, eventID, tiket[0].kode, petugas, diPintu(18, 0))
		hasil <- err
	}()

	// check-in harus sedang menunggu kunci, belum selesai
	select {
	case err := <-hasil:
		t.Fatalf("check-in selesai sebelum pemindahan commit: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	// 3. pemindahan selesai
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// 4. check-in dilanjutkan dan menolak kode lama dengan pesan yang jelas
	err = <-hasil
	var salah KesalahanInput
	if !errors.As(err, &salah) || !strings.Contains(salah.Pesan, "sudah dipindahkan ke Rina Penerima") {
		t.Fatalf("mau ditolak 'sudah dipindahkan ke Rina Penerima', dapat %v", err)
	}
	if s := statusTiket(t, db, tiket[0].id); s != "ACTIVE" {
		t.Errorf("status tiket %s, mau tetap ACTIVE (belum ada yang masuk)", s)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM checkins"); n != 0 {
		t.Errorf("ada %d checkins dari kode lama", n)
	}
}
