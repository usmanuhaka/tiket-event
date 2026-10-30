package checkin

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/event"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// pembaca adalah kemampuan membaca satu baris. *sql.DB dan *sql.Tx sama-sama memilikinya,
// jadi bacaTiket bisa dipakai di halaman periksa (tanpa transaksi) maupun di dalam CheckIn.
type pembaca interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ambilEventHariIni mengambil event OPEN yang tanggalnya hari ini (WIB), untuk langkah 1 petugas
func ambilEventHariIni(ctx context.Context, db *sql.DB, now time.Time) ([]event.Event, error) {
	n := now.In(web.Jakarta)
	awal := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, web.Jakarta)
	akhir := awal.AddDate(0, 0, 1)

	rows, err := db.QueryContext(ctx,
		`SELECT id, nama, lokasi, tanggal_event
		FROM events
		WHERE status = ? AND deleted_at IS NULL AND tanggal_event >= ? AND tanggal_event < ?
		ORDER BY tanggal_event`, event.StatusOpen, awal, akhir)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []event.Event
	for rows.Next() {
		var e event.Event
		if err := rows.Scan(&e.ID, &e.Nama, &e.Lokasi, &e.TanggalEvent); err != nil {
			return nil, err
		}
		hasil = append(hasil, e)
	}
	return hasil, rows.Err()
}

// bacaTiket membaca data tampilan tiket (BR-24) berdasarkan id, tanpa mengunci apa pun
func bacaTiket(ctx context.Context, q pembaca, tiketID int64) (InfoTiket, error) {
	var t InfoTiket
	var waktuMasuk sql.NullTime
	err := q.QueryRowContext(ctx,
		`SELECT t.id, t.kode_tiket, o.event_id, e.nama, e.tanggal_event, t.nama_pemegang, c.nama, t.status, ci.waktu_masuk
		FROM tickets t
		JOIN orders o ON o.id = t.order_id
		JOIN events e ON e.id = o.event_id
		JOIN ticket_categories c ON c.id = t.category_id
		LEFT JOIN checkins ci ON ci.ticket_id = t.id
		WHERE t.id = ?`, tiketID,
	).Scan(&t.ID, &t.Kode, &t.EventID, &t.NamaEvent, &t.TanggalEvent, &t.NamaPemegang, &t.NamaKategori, &t.Status, &waktuMasuk)
	if waktuMasuk.Valid {
		t.WaktuMasuk = waktuMasuk.Time
	}
	return t, err
}

// tolakKodeTakDikenal dipanggil jika kode tidak ada di tickets.kode_tiket.
// Baru sekarang ticket_transfers.kode_lama diperiksa (kode yang berlaku selalu didahulukan).
func tolakKodeTakDikenal(ctx context.Context, q pembaca, kode string) error {
	var namaPenerima string
	err := q.QueryRowContext(ctx, "SELECT nama_baru FROM ticket_transfers WHERE kode_lama = ?", kode).Scan(&namaPenerima)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrKodeTidakDitemukan
	}
	if err != nil {
		return err
	}
	return PesanKodeLama(namaPenerima)
}

// periksaKode dipakai halaman langkah 2 (hanya membaca): cari kode, lalu tampilkan data tiketnya.
// Hasil boleh/tidaknya masuk dihitung handler dengan CekMasuk.
func periksaKode(ctx context.Context, db *sql.DB, kode string) (InfoTiket, error) {
	kode = NormalisasiKode(kode)

	var id int64
	err := db.QueryRowContext(ctx, "SELECT id FROM tickets WHERE kode_tiket = ?", kode).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return InfoTiket{}, tolakKodeTakDikenal(ctx, db, kode)
	}
	if err != nil {
		return InfoTiket{}, err
	}
	return bacaTiket(ctx, db, id)
}

// CheckIn (BR-25, BR-26) mengizinkan tiket masuk: tiket menjadi USED dan satu baris checkins dicatat.
//
// Urutan kunci sama dengan alur lain: event (yang sedang dijaga petugas) -> tiket.
// Mengunci tiket saja TIDAK cukup: INSERT ke checkins memeriksa foreign key event_id, dan pemeriksaan
// itu memasang kunci baca pada baris event. Jika event belum dikunci lebih dulu, check-in bisa memegang
// kunci tiket sambil menunggu baris event, sementara pembatalan/pemindahan memegang baris event sambil
// menunggu tiket -> deadlock. Test konkurensi di paket ticket membuktikannya.
//
// Kunci tiket diambil dengan SELECT tanpa JOIN, karena SELECT ... JOIN ... FOR UPDATE akan ikut
// mengunci baris pesanan dan kategori.
//
// Diekspor supaya test di paket ticket bisa menguji check-in yang bersamaan dengan
// pembatalan dan pemindahan tiket.
func CheckIn(ctx context.Context, db *sql.DB, eventID int64, kode string, petugasID int64, now time.Time) (InfoTiket, error) {
	kode = NormalisasiKode(kode)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return InfoTiket{}, err
	}
	defer tx.Rollback()

	// 1. kunci baris event yang sedang dijaga petugas
	if _, err := event.KunciEvent(ctx, tx, eventID); err != nil {
		if errors.Is(err, event.ErrTidakDitemukan) {
			return InfoTiket{}, salahInput("Event tidak ditemukan.")
		}
		return InfoTiket{}, err
	}

	// 2. kunci baris tiket berdasarkan kode yang berlaku
	var id int64
	err = tx.QueryRowContext(ctx, "SELECT id FROM tickets WHERE kode_tiket = ? FOR UPDATE", kode).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// kode tidak berlaku. Jika kode ini baru saja diganti oleh pemindahan yang sedang berjalan,
		// kunci di atas menunggu pemindahan itu selesai, lalu kode lama sudah tercatat di ticket_transfers.
		return InfoTiket{}, tolakKodeTakDikenal(ctx, tx, kode)
	}
	if err != nil {
		return InfoTiket{}, err
	}

	// 3. baca keadaan tiket terbaru (sudah terkunci, jadi tidak bisa berubah sampai COMMIT)
	t, err := bacaTiket(ctx, tx, id)
	if err != nil {
		return t, err
	}

	// 4. BR-25 dan BR-26 diperiksa ulang pada keadaan terkunci
	if err := CekMasuk(t, eventID, now); err != nil {
		return t, err
	}

	// 5. tiket dipakai masuk dan waktunya dicatat
	if _, err := tx.ExecContext(ctx,
		"UPDATE tickets SET status = ?, updated_at = NOW() WHERE id = ?", tiketUsed, id,
	); err != nil {
		return t, err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO checkins (ticket_id, event_id, petugas_id, waktu_masuk) VALUES (?, ?, ?, ?)",
		id, eventID, petugasID, now,
	); err != nil {
		return t, err
	}
	if err := tx.Commit(); err != nil {
		return t, err
	}

	t.Status = tiketUsed
	t.WaktuMasuk = now
	return t, nil
}
