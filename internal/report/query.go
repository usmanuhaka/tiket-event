package report

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrTidakDitemukan dikembalikan jika event tidak ada atau sudah dihapus
var ErrTidakDitemukan = errors.New("Event tidak ditemukan")

// pembaca adalah kemampuan menjalankan SELECT. *sql.Tx memilikinya; test memakainya
// untuk membaca langkah demi langkah di dalam satu transaksi.
type pembaca interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// opsiSnapshot: transaksi hanya-baca dengan isolasi REPEATABLE READ.
// Di InnoDB, semua SELECT biasa di dalam transaksi ini membaca dari satu snapshot
// yang dibuat pada SELECT pertama, tanpa mengunci baris apa pun.
var opsiSnapshot = &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead}

// AmbilLaporan (BR-27, BR-28) menyusun laporan satu event dari satu snapshot yang konsisten,
// walaupun pemesanan, pembayaran, pembatalan, atau check-in sedang berjalan.
// Diekspor supaya test di paket ticket bisa memeriksa laporan setelah pembatalan sungguhan.
func AmbilLaporan(ctx context.Context, db *sql.DB, eventID int64) (Laporan, error) {
	tx, err := db.BeginTx(ctx, opsiSnapshot)
	if err != nil {
		return Laporan{}, err
	}
	defer tx.Rollback() // transaksi baca: tidak ada yang perlu disimpan

	// SELECT pertama ini sekaligus menetapkan snapshot
	lap, err := bacaEvent(ctx, tx, eventID)
	if err != nil {
		return lap, err
	}

	// baris per tahap dibaca dari snapshot yang sama
	baris, err := bacaBarisTahap(ctx, tx, eventID)
	if err != nil {
		return lap, err
	}

	// subtotal kategori dan total event dijumlahkan dari baris yang sama
	lap.Kategori, lap.Total = SusunLaporan(baris)
	return lap, tx.Commit()
}

func bacaEvent(ctx context.Context, q pembaca, eventID int64) (Laporan, error) {
	lap := Laporan{EventID: eventID}
	err := q.QueryRowContext(ctx,
		"SELECT nama, tanggal_event, status FROM events WHERE id = ? AND deleted_at IS NULL", eventID,
	).Scan(&lap.NamaEvent, &lap.TanggalEvent, &lap.StatusEvent)
	if errors.Is(err, sql.ErrNoRows) {
		return lap, ErrTidakDitemukan
	}
	return lap, err
}

// bacaBarisTahap menghitung angka laporan untuk setiap tahap harga event dalam satu query.
//   - LEFT JOIN tickets: tahap yang belum terjual tetap muncul dengan angka 0.
//     Syarat status ditulis di ON (bukan WHERE), karena di WHERE tahap tanpa penjualan akan hilang.
//   - "terjual" = ACTIVE, USED, REFUNDED; HELD dan CANCELLED tidak dihitung.
//   - LEFT JOIN refunds: satu tiket paling banyak satu refund (refunds.ticket_id UNIQUE),
//     jadi tidak ada baris tiket yang terhitung dua kali.
//   - tahap berkuota 0 (kuotanya sudah dipindah) tetap tampil jika punya penjualan.
func bacaBarisTahap(ctx context.Context, q pembaca, eventID int64) ([]BarisTahap, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT c.id, c.nama, pt.id, pt.nama, pt.urutan, pt.harga, pt.kuota, pt.sisa_kuota,
			COUNT(t.id),
			COUNT(r.id),
			COALESCE(SUM(CASE WHEN t.status = 'USED' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(t.harga), 0),
			COALESCE(SUM(t.potongan), 0),
			COALESCE(SUM(r.jumlah), 0)
		FROM ticket_categories c
		JOIN price_tiers pt ON pt.category_id = c.id AND pt.deleted_at IS NULL
		LEFT JOIN tickets t ON t.price_tier_id = pt.id AND t.status IN ('ACTIVE', 'USED', 'REFUNDED')
		LEFT JOIN refunds r ON r.ticket_id = t.id
		WHERE c.event_id = ? AND c.deleted_at IS NULL
		GROUP BY c.id, c.nama, pt.id, pt.nama, pt.urutan, pt.harga, pt.kuota, pt.sisa_kuota
		HAVING pt.kuota > 0 OR COUNT(t.id) > 0
		ORDER BY c.id, pt.urutan`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []BarisTahap
	for rows.Next() {
		var b BarisTahap
		if err := rows.Scan(&b.KategoriID, &b.NamaKategori, &b.TahapID, &b.NamaTahap, &b.Urutan, &b.Harga, &b.Kuota, &b.SisaKuota,
			&b.Terjual, &b.TiketRefund, &b.SudahMasuk, &b.Kotor, &b.Potongan, &b.Refund); err != nil {
			return nil, err
		}
		hasil = append(hasil, b)
	}
	return hasil, rows.Err()
}

// RingkasanEvent adalah satu baris di halaman daftar laporan
type RingkasanEvent struct {
	ID           int64
	Nama         string
	TanggalEvent time.Time
	Status       string
	Angka
}

// ambilRingkasanSemuaEvent menghitung total setiap event non-DRAFT dalam satu query
// (satu query = satu snapshot), untuk halaman daftar laporan
func ambilRingkasanSemuaEvent(ctx context.Context, db *sql.DB) ([]RingkasanEvent, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT e.id, e.nama, e.tanggal_event, e.status,
			COUNT(t.id),
			COUNT(r.id),
			COALESCE(SUM(CASE WHEN t.status = 'USED' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(t.harga), 0),
			COALESCE(SUM(t.potongan), 0),
			COALESCE(SUM(r.jumlah), 0)
		FROM events e
		LEFT JOIN orders o ON o.event_id = e.id
		LEFT JOIN tickets t ON t.order_id = o.id AND t.status IN ('ACTIVE', 'USED', 'REFUNDED')
		LEFT JOIN refunds r ON r.ticket_id = t.id
		WHERE e.deleted_at IS NULL AND e.status <> 'DRAFT'
		GROUP BY e.id, e.nama, e.tanggal_event, e.status
		ORDER BY e.tanggal_event DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []RingkasanEvent
	for rows.Next() {
		var r RingkasanEvent
		if err := rows.Scan(&r.ID, &r.Nama, &r.TanggalEvent, &r.Status,
			&r.Terjual, &r.TiketRefund, &r.SudahMasuk, &r.Kotor, &r.Potongan, &r.Refund); err != nil {
			return nil, err
		}
		hasil = append(hasil, r)
	}
	return hasil, rows.Err()
}
