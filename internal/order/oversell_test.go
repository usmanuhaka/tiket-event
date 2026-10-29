package order

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
)

// BR-12: tidak boleh oversell walaupun banyak orang memesan bersamaan.
//
// pesanBersamaan menjalankan `jumlahOrang` goroutine yang masing-masing memesan `tiketPerOrang` tiket
// pada saat yang sama, lalu mengembalikan berapa pesanan yang berhasil.
// Setiap penolakan harus berupa KesalahanInput (kuota tidak cukup), bukan error database.
func pesanBersamaan(t *testing.T, db *sql.DB, e eventUji, jumlahOrang, tiketPerOrang int) int {
	t.Helper()
	userID := buatPembeli(t, db)

	var wg sync.WaitGroup
	var mu sync.Mutex
	berhasil := 0
	mulai := make(chan struct{}) // semua goroutine menunggu aba-aba yang sama

	for i := 0; i < jumlahOrang; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-mulai
			_, err := buatPesanan(context.Background(), db, userID, e.eventID, pesan(e.kategoriID, tiketPerOrang), waktuSekarang())
			if err == nil {
				mu.Lock()
				berhasil++
				mu.Unlock()
				return
			}
			var salah KesalahanInput
			if !errors.As(err, &salah) {
				t.Errorf("error selain kuota habis: %v", err)
			}
		}()
	}
	close(mulai)
	wg.Wait()
	return berhasil
}

func TestBR12LimaPuluhOrangBerebutSepuluhTiket(t *testing.T) {
	db := bukaDBKosong(t)
	// kuota 10: Presale 5 + Normal 5
	e := siapkanEvent(t, db, []tahapUji{{"Presale", 100000, 5, 5}, {"Normal", 150000, 5, 20}})

	berhasil := pesanBersamaan(t, db, e, 50, 1)

	if berhasil != 10 {
		t.Errorf("berhasil %d pesanan, mau tepat 10", berhasil)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM tickets"); n != 10 {
		t.Errorf("jumlah tiket %d, mau 10", n)
	}
	if n := hitung(t, db, "SELECT SUM(sisa_kuota) FROM price_tiers WHERE category_id = ?", e.kategoriID); n != 0 {
		t.Errorf("sisa kuota %d, mau 0", n)
	}
	// alokasi berurutan tetap benar: 5 tiket Presale dulu, baru 5 Normal
	for i, id := range e.tahapID {
		if n := hitung(t, db, "SELECT COUNT(*) FROM tickets WHERE price_tier_id = ?", id); n != 5 {
			t.Errorf("tahap ke-%d terjual %d, mau 5", i+1, n)
		}
	}
}

func TestBR12PesananTigaTiketTidakPernahSebagian(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 10, 20}})

	// 20 orang masing-masing memesan 3 tiket, kuota hanya 10: muat 3 pesanan (9 tiket)
	berhasil := pesanBersamaan(t, db, e, 20, 3)

	if berhasil != 3 {
		t.Errorf("berhasil %d pesanan, mau 3", berhasil)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM tickets"); n != 9 {
		t.Errorf("jumlah tiket %d, mau 9", n)
	}
	if n := hitung(t, db, "SELECT sisa_kuota FROM price_tiers WHERE id = ?", e.tahapID[0]); n != 1 {
		t.Errorf("sisa kuota %d, mau 1", n)
	}
	// setiap pesanan berisi tepat 3 tiket, tidak ada yang setengah jadi
	if n := hitung(t, db, "SELECT COUNT(*) FROM (SELECT order_id FROM tickets GROUP BY order_id HAVING COUNT(*) <> 3) x"); n != 0 {
		t.Errorf("ada %d pesanan yang jumlah tiketnya bukan 3", n)
	}
}
