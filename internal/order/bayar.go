package order

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/usmanuhaka/tiket-event/internal/event"
	"github.com/usmanuhaka/tiket-event/internal/promo"
)

// ErrWaktuBayarHabis dikembalikan jika pembeli menekan "Konfirmasi Pembayaran" setelah 15 menit
var ErrWaktuBayarHabis = KesalahanInput{Pesan: "Waktu pembayaran 15 menit sudah habis. Pesanan dibatalkan dan tiket dilepas kembali."}

// kodeDuplikat adalah nomor error MySQL/MariaDB untuk pelanggaran UNIQUE
const kodeDuplikat = 1062

// ===== Pembayaran (BR-11) =====

// bayarPesanan mensimulasikan pembayaran. Jika masih dalam 15 menit, pesanan menjadi PAID,
// tiket menjadi ACTIVE, dan setiap tiket mendapat kode unik.
// Jika sudah lewat, pesanan langsung dibuat EXPIRED di sini (BR-10: dicek ulang saat pembayaran).
func bayarPesanan(ctx context.Context, db *sql.DB, orderID, userID int64, now time.Time) error {
	eventID, pemilik, err := eventPesanan(ctx, db, orderID)
	if err != nil {
		return err
	}
	if pemilik != userID {
		return ErrTidakDitemukan // pesanan orang lain dianggap tidak ada
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	status, batasBayar, err := kunciPesanan(ctx, tx, orderID, eventID)
	if err != nil {
		return err
	}
	switch status {
	case StatusPaid:
		return salahInput("Pesanan ini sudah dibayar")
	case StatusExpired:
		return salahInput("Pesanan ini sudah kedaluwarsa")
	case StatusCancelled:
		return salahInput("Pesanan ini sudah dibatalkan")
	}

	// BR-10: walaupun worker belum sempat jalan, batas waktu dicek ulang di sini
	if SudahKedaluwarsa(batasBayar, now) {
		if err := kedaluwarsakan(ctx, tx, orderID, now); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return ErrWaktuBayarHabis
	}

	// BR-11: setiap tiket mendapat kode unik dan menjadi ACTIVE
	idTiket, err := tiketHeld(ctx, tx, orderID)
	if err != nil {
		return err
	}
	for _, t := range idTiket {
		if err := beriKodeTiket(ctx, tx, t.id); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE orders SET status = ?, paid_at = ?, updated_at = NOW() WHERE id = ?",
		StatusPaid, now, orderID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// beriKodeTiket memberi kode acak ke satu tiket dan mengubahnya menjadi ACTIVE.
// Jika kode kebetulan sudah dipakai tiket lain (kolom UNIQUE menolak), dibuat kode baru, maksimal 3 kali.
func beriKodeTiket(ctx context.Context, tx *sql.Tx, tiketID int64) error {
	for percobaan := 1; percobaan <= 3; percobaan++ {
		_, err := tx.ExecContext(ctx,
			"UPDATE tickets SET kode_tiket = ?, status = ?, updated_at = NOW() WHERE id = ?",
			BuatKodeTiket(), TiketActive, tiketID,
		)
		if err == nil {
			return nil
		}
		var errMysql *mysql.MySQLError
		if !errors.As(err, &errMysql) || errMysql.Number != kodeDuplikat {
			return err
		}
		// kode sama dengan tiket lain: ulangi dengan kode baru
	}
	return errors.New("gagal membuat kode tiket yang unik")
}

// ===== Kedaluwarsa (BR-10) =====

// ExpireKedaluwarsa dipanggil worker setiap 1 menit. Pesanan HELD yang batas bayarnya sudah lewat
// dibuat EXPIRED dan kuotanya dikembalikan. Mengembalikan jumlah pesanan yang di-expire.
func ExpireKedaluwarsa(ctx context.Context, db *sql.DB, now time.Time) (int, error) {
	// 1. cari kandidat (tanpa kunci, hanya membaca)
	rows, err := db.QueryContext(ctx,
		"SELECT id FROM orders WHERE status = ? AND expires_at < ? ORDER BY id", StatusHeld, now)
	if err != nil {
		return 0, err
	}
	var kandidat []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		kandidat = append(kandidat, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	// 2. proses satu per satu, masing-masing dalam transaksinya sendiri
	jumlah := 0
	for _, id := range kandidat {
		diproses, err := expirePesanan(ctx, db, id, now)
		if err != nil {
			return jumlah, err
		}
		if diproses {
			jumlah++
		}
	}
	return jumlah, nil
}

// expirePesanan membuat satu pesanan EXPIRED jika (setelah dikunci) masih HELD dan sudah lewat waktu.
// Pengecekan ulang perlu karena pembeli bisa saja membayar di antara pencarian dan penguncian.
func expirePesanan(ctx context.Context, db *sql.DB, orderID int64, now time.Time) (bool, error) {
	eventID, _, err := eventPesanan(ctx, db, orderID)
	if err != nil {
		return false, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	status, batasBayar, err := kunciPesanan(ctx, tx, orderID, eventID)
	if err != nil {
		return false, err
	}
	if status != StatusHeld || !SudahKedaluwarsa(batasBayar, now) {
		return false, nil // sudah dibayar atau belum waktunya: tidak diubah
	}

	if err := kedaluwarsakan(ctx, tx, orderID, now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// kedaluwarsakan mengembalikan kuota setiap tiket HELD lalu mengubah status pesanan menjadi EXPIRED
// dan tiket menjadi CANCELLED. Dipanggil di dalam transaksi yang sudah mengunci event, pesanan, dan tahap.
// Jika pesanan memakai promo, baris promo dikunci di sini (paling akhir) untuk mengembalikan pemakaiannya.
func kedaluwarsakan(ctx context.Context, tx *sql.Tx, orderID int64, now time.Time) error {
	daftar, err := tiketHeld(ctx, tx, orderID)
	if err != nil {
		return err
	}

	// BR-10: kuota dikembalikan per tiket. Ke tahap asal jika masih berlaku,
	// atau dipindah ke tahap berikutnya jika tahap asal sudah lewat (keputusan di PRD §9).
	for _, t := range daftar {
		if _, err := event.KembalikanKuota(ctx, tx, t.kategoriID, t.tahapID, now); err != nil {
			return err
		}
	}

	// BR-15: pemakaian promo dikembalikan karena pesanan EXPIRED
	var promoID sql.NullInt64
	if err := tx.QueryRowContext(ctx, "SELECT promo_code_id FROM orders WHERE id = ?", orderID).Scan(&promoID); err != nil {
		return err
	}
	if promoID.Valid {
		if err := promo.KembalikanPemakaian(ctx, tx, promoID.Int64); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE tickets SET status = ?, updated_at = NOW() WHERE order_id = ? AND status = ?",
		TiketCancelled, orderID, TiketHeld,
	); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE orders SET status = ?, updated_at = NOW() WHERE id = ?", StatusExpired, orderID)
	return err
}

// ===== Helper penguncian =====

// eventPesanan membaca event dan pemilik sebuah pesanan (tanpa kunci; keduanya tidak pernah berubah)
func eventPesanan(ctx context.Context, db *sql.DB, orderID int64) (eventID, userID int64, err error) {
	err = db.QueryRowContext(ctx, "SELECT event_id, user_id FROM orders WHERE id = ?", orderID).Scan(&eventID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrTidakDitemukan
	}
	return eventID, userID, err
}

// kunciPesanan mengunci sesuai urutan wajib: baris event -> baris pesanan -> semua tahap
// kategori tiket pesanan ini (ORDER BY id). Mengembalikan status dan batas bayar terbaru.
func kunciPesanan(ctx context.Context, tx *sql.Tx, orderID, eventID int64) (string, time.Time, error) {
	var status string
	var batasBayar time.Time

	// 1. event
	if _, err := event.KunciEvent(ctx, tx, eventID); err != nil {
		return status, batasBayar, err
	}

	// 2. pesanan
	err := tx.QueryRowContext(ctx,
		"SELECT status, expires_at FROM orders WHERE id = ? FOR UPDATE", orderID,
	).Scan(&status, &batasBayar)
	if errors.Is(err, sql.ErrNoRows) {
		return status, batasBayar, ErrTidakDitemukan
	}
	if err != nil {
		return status, batasBayar, err
	}

	// 3. tahap dari semua kategori tiket pesanan ini
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT category_id FROM tickets WHERE order_id = ?", orderID)
	if err != nil {
		return status, batasBayar, err
	}
	var kategori []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return status, batasBayar, err
		}
		kategori = append(kategori, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return status, batasBayar, err
	}

	_, err = event.KunciTahapKategori(ctx, tx, kategori)
	return status, batasBayar, err
}

type tiketDitahan struct {
	id         int64
	kategoriID int64
	tahapID    int64
}

// tiketHeld membaca tiket HELD sebuah pesanan.
// Semua baris dibaca dulu sampai selesai, baru kemudian di-UPDATE oleh pemanggil
// (satu koneksi transaksi tidak bisa menjalankan UPDATE selagi hasil SELECT masih terbuka).
func tiketHeld(ctx context.Context, tx *sql.Tx, orderID int64) ([]tiketDitahan, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, category_id, price_tier_id FROM tickets WHERE order_id = ? AND status = ? ORDER BY id",
		orderID, TiketHeld)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []tiketDitahan
	for rows.Next() {
		var t tiketDitahan
		if err := rows.Scan(&t.id, &t.kategoriID, &t.tahapID); err != nil {
			return nil, err
		}
		hasil = append(hasil, t)
	}
	return hasil, rows.Err()
}
