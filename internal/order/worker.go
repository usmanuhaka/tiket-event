package order

import (
	"context"
	"database/sql"
	"log"
	"time"
)

// JalankanWorker (BR-10) memeriksa pesanan kedaluwarsa sekali saat mulai, lalu setiap `interval`
// (di main: 1 menit). Dijalankan sebagai goroutine di samping server web dan berhenti saat ctx dibatalkan.
func JalankanWorker(ctx context.Context, db *sql.DB, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		periksaSekali(ctx, db)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// tunggu giliran berikutnya
		}
	}
}

// periksaSekali menjalankan satu putaran worker. Error hanya dicatat di log supaya
// worker tetap hidup dan mencoba lagi pada putaran berikutnya.
func periksaSekali(ctx context.Context, db *sql.DB) {
	jumlah, err := ExpireKedaluwarsa(ctx, db, time.Now())
	if err != nil {
		log.Printf("worker kedaluwarsa: %v", err)
	}
	if jumlah > 0 {
		log.Printf("worker kedaluwarsa: %d pesanan dibuat EXPIRED", jumlah)
	}
}
