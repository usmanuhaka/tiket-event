package ticket

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/event"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// ErrTidakDitemukan dikembalikan jika tiket/event tidak ada, atau tiket bukan milik pembeli yang login
var ErrTidakDitemukan = errors.New("Data tidak ditemukan")

// Status pesanan dan tiket yang dipakai paket ini (sama dengan ENUM di database).
// Ditulis ulang di sini supaya paket ticket tidak bergantung pada paket order.
const (
	pesananHeld      = "HELD"
	pesananCancelled = "CANCELLED"
	tiketHeld        = "HELD"
	tiketActive      = "ACTIVE"
	tiketCancelled   = "CANCELLED"
	tiketRefunded    = "REFUNDED"
)

// HasilBatal adalah hasil pembatalan satu tiket oleh pembeli
type HasilBatal struct {
	OrderID int64
	Refund  int64
}

// ===== Pembatalan tiket oleh pembeli (BR-17, BR-18, BR-19) =====

// batalkanTiket membatalkan satu tiket ACTIVE milik pembeli userID.
// Urutan kunci sama dengan alur lain: event -> pesanan -> tiket -> tahap.
func batalkanTiket(ctx context.Context, db *sql.DB, tiketID, userID int64, now time.Time) (HasilBatal, error) {
	var hasil HasilBatal

	// 1. baca event, pesanan, dan pemilik tiket (tanpa kunci; nilainya tidak pernah berubah)
	var eventID, pemilik int64
	err := db.QueryRowContext(ctx,
		`SELECT t.order_id, o.event_id, o.user_id
		FROM tickets t JOIN orders o ON o.id = t.order_id
		WHERE t.id = ?`, tiketID,
	).Scan(&hasil.OrderID, &eventID, &pemilik)
	if errors.Is(err, sql.ErrNoRows) {
		return hasil, ErrTidakDitemukan
	}
	if err != nil {
		return hasil, err
	}
	// hanya pembeli pemilik pesanan yang boleh membatalkan (keputusan di PRD §9)
	if pemilik != userID {
		return hasil, ErrTidakDitemukan
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return hasil, err
	}
	defer tx.Rollback()

	// 2. kunci event
	e, err := event.KunciEvent(ctx, tx, eventID)
	if errors.Is(err, event.ErrTidakDitemukan) {
		return hasil, ErrTidakDitemukan
	}
	if err != nil {
		return hasil, err
	}
	if e.Status == event.StatusCancelled {
		return hasil, salahInput("Event sudah dibatalkan penyelenggara; refund 100%% sudah diproses otomatis")
	}
	if e.Status != event.StatusOpen {
		return hasil, salahInput("Event ini tidak sedang dibuka")
	}

	// 3. kunci pesanan
	var kunciPesanan int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM orders WHERE id = ? FOR UPDATE", hasil.OrderID).Scan(&kunciPesanan); err != nil {
		return hasil, err
	}

	// 4. kunci tiket, lalu periksa statusnya (menolak pembatalan ganda)
	var kategoriID, tahapID, hargaBayar int64
	var status string
	if err := tx.QueryRowContext(ctx,
		"SELECT category_id, price_tier_id, harga_bayar, status FROM tickets WHERE id = ? FOR UPDATE", tiketID,
	).Scan(&kategoriID, &tahapID, &hargaBayar, &status); err != nil {
		return hasil, err
	}
	if status != tiketActive {
		return hasil, salahInput("Tiket ini tidak bisa dibatalkan (status %s)", status)
	}

	// BR-17: paling lambat H-7, dihitung per tanggal kalender Jakarta
	if !BolehBatal(e.TanggalEvent, now) {
		return hasil, salahInput("Pembatalan paling lambat %s (7 hari sebelum event)", web.TanggalSaja(BatasBatal(e.TanggalEvent)))
	}

	// 5. kunci tahap kategori tiket, lalu kembalikan kuotanya (BR-19).
	// Tahap asal yang sudah lewat memindahkan kuota ke tahap berikutnya (PRD §9), jadi kursinya bisa dijual lagi.
	if _, err := event.KunciTahapKategori(ctx, tx, []int64{kategoriID}); err != nil {
		return hasil, err
	}
	if _, err := event.KembalikanKuota(ctx, tx, kategoriID, tahapID, now); err != nil {
		return hasil, err
	}

	// 6. BR-18: refund 75% dari harga yang dibayar (setelah promo), dibulatkan ke bawah.
	// Pemakaian promo TIDAK dikembalikan (PRD §9).
	hasil.Refund = HitungRefund(hargaBayar, PersenRefundPembeli)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO refunds (ticket_id, order_id, user_id, jumlah, persen, alasan)
		VALUES (?, ?, ?, ?, ?, ?)`,
		tiketID, hasil.OrderID, pemilik, hasil.Refund, PersenRefundPembeli, AlasanPembeli,
	); err != nil {
		return hasil, err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE tickets SET status = ?, updated_at = NOW() WHERE id = ?", tiketRefunded, tiketID,
	); err != nil {
		return hasil, err
	}

	return hasil, tx.Commit()
}
