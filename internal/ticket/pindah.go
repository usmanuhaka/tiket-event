package ticket

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/usmanuhaka/tiket-event/internal/event"
)

// kodeDuplikat adalah nomor error MySQL/MariaDB untuk pelanggaran UNIQUE
const kodeDuplikat = 1062

// TiketPindah adalah data tiket untuk form pemindahan
type TiketPindah struct {
	ID           int64
	OrderID      int64
	NamaEvent    string
	NamaKategori string
	NamaPemegang string
}

// HasilPindah adalah hasil pemindahan satu tiket
type HasilPindah struct {
	OrderID  int64
	KodeLama string
	KodeBaru string
}

// ambilTiketMilik mengambil data tiket untuk form pemindahan, hanya jika milik pemilik pesanan userID
func ambilTiketMilik(ctx context.Context, db *sql.DB, tiketID, userID int64) (TiketPindah, error) {
	var t TiketPindah
	err := db.QueryRowContext(ctx,
		`SELECT t.id, t.order_id, e.nama, c.nama, t.nama_pemegang
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		JOIN events e ON e.id = o.event_id
		JOIN ticket_categories c ON c.id = t.category_id
		WHERE t.id = ? AND o.user_id = ?`, tiketID, userID,
	).Scan(&t.ID, &t.OrderID, &t.NamaEvent, &t.NamaKategori, &t.NamaPemegang)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrTidakDitemukan
	}
	return t, err
}

// pindahkanTiket (BR-21, BR-22, BR-23) mengganti nama pemegang dan kode tiket.
// Urutan kunci sama dengan alur lain: event -> pesanan -> tiket.
// Kuota tidak berubah, jadi tahap dan promo tidak perlu dikunci.
func pindahkanTiket(ctx context.Context, db *sql.DB, tiketID, userID int64, namaBaru string, now time.Time) (HasilPindah, error) {
	var hasil HasilPindah

	// 1. baca event, pesanan, dan pemilik (tanpa kunci; nilainya tidak pernah berubah)
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
	// hanya pemilik pesanan yang boleh memindahkan (PRD §9)
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

	// 3. kunci pesanan
	var kunciPesanan int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM orders WHERE id = ? FOR UPDATE", hasil.OrderID).Scan(&kunciPesanan); err != nil {
		return hasil, err
	}

	// 4. kunci tiket, lalu periksa syarat BR-21 dan BR-23 pada keadaan terbaru
	var namaLama, status string
	var kodeLama sql.NullString
	var sudahDipindah bool
	if err := tx.QueryRowContext(ctx,
		"SELECT nama_pemegang, kode_tiket, status, sudah_dipindah FROM tickets WHERE id = ? FOR UPDATE", tiketID,
	).Scan(&namaLama, &kodeLama, &status, &sudahDipindah); err != nil {
		return hasil, err
	}
	if err := CekBolehPindah(status, sudahDipindah, e.Status, e.TanggalEvent, now); err != nil {
		return hasil, err
	}
	if err := ValidasiNamaBaru(namaLama, namaBaru); err != nil {
		return hasil, err
	}
	hasil.KodeLama = kodeLama.String
	namaBaru = strings.TrimSpace(namaBaru)

	// 5. BR-22: kode baru. Jika kebetulan sama dengan kode tiket lain (UNIQUE menolak), buat ulang.
	for percobaan := 1; ; percobaan++ {
		hasil.KodeBaru = BuatKodeTiket()
		_, err := tx.ExecContext(ctx,
			`UPDATE tickets SET nama_pemegang = ?, kode_tiket = ?, sudah_dipindah = 1, updated_at = NOW()
			WHERE id = ?`, namaBaru, hasil.KodeBaru, tiketID)
		if err == nil {
			break
		}
		var errMysql *mysql.MySQLError
		if !errors.As(err, &errMysql) || errMysql.Number != kodeDuplikat || percobaan == 3 {
			return hasil, err
		}
	}

	// 6. catat pemindahan; kode lama disimpan supaya pemeriksaan di pintu bisa menjelaskannya
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO ticket_transfers (ticket_id, user_id, nama_lama, nama_baru, kode_lama, kode_baru)
		VALUES (?, ?, ?, ?, ?, ?)`,
		tiketID, userID, namaLama, namaBaru, hasil.KodeLama, hasil.KodeBaru,
	); err != nil {
		return hasil, err
	}

	return hasil, tx.Commit()
}
