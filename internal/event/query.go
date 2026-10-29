package event

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrTidakDitemukan dikembalikan jika event, kategori, atau tahap tidak ada (atau sudah dihapus)
var ErrTidakDitemukan = errors.New("Data tidak ditemukan")

// querier adalah kemampuan menjalankan SELECT banyak baris.
// *sql.DB dan *sql.Tx sama-sama memilikinya, jadi AmbilKategoriDanTahap
// bisa dipakai di luar maupun di dalam transaksi.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ===== Baca data (tanpa transaksi) =====

func ambilSemuaEvent(ctx context.Context, db *sql.DB) ([]Event, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, nama, lokasi, COALESCE(deskripsi, ''), tanggal_event, kapasitas, status
		FROM events WHERE deleted_at IS NULL ORDER BY tanggal_event`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Nama, &e.Lokasi, &e.Deskripsi, &e.TanggalEvent, &e.Kapasitas, &e.Status); err != nil {
			return nil, err
		}
		hasil = append(hasil, e)
	}
	return hasil, rows.Err()
}

// AmbilEventDibuka mengambil event yang bisa dipesan pembeli:
// status OPEN dan tanggalnya belum lewat. Dipakai halaman katalog di paket order.
func AmbilEventDibuka(ctx context.Context, db *sql.DB, now time.Time) ([]Event, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, nama, lokasi, COALESCE(deskripsi, ''), tanggal_event, kapasitas, status
		FROM events
		WHERE status = ? AND deleted_at IS NULL AND tanggal_event > ?
		ORDER BY tanggal_event`, StatusOpen, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Nama, &e.Lokasi, &e.Deskripsi, &e.TanggalEvent, &e.Kapasitas, &e.Status); err != nil {
			return nil, err
		}
		hasil = append(hasil, e)
	}
	return hasil, rows.Err()
}

func GetEventById(ctx context.Context, db *sql.DB, id int64) (Event, error) {
	var e Event

	err := db.QueryRowContext(ctx,
		`SELECT id, nama, lokasi, COALESCE(deskripsi, ''), tanggal_event, kapasitas, status
		FROM events WHERE id = ? AND deleted_at IS NULL`, id,
	).Scan(&e.ID, &e.Nama, &e.Lokasi, &e.Deskripsi, &e.TanggalEvent, &e.Kapasitas, &e.Status)

	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrTidakDitemukan
	}
	return e, err
}

// AmbilKategoriDanTahap mengambil semua kategori sebuah event beserta tahapnya (urut sesuai kolom urutan)
func AmbilKategoriDanTahap(ctx context.Context, q querier, eventID int64) ([]Kategori, error) {
	// 1. ambil kategori
	rows, err := q.QueryContext(ctx,
		"SELECT id, event_id, nama, kuota FROM ticket_categories WHERE event_id = ? AND deleted_at IS NULL ORDER BY id",
		eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kategori []Kategori
	posisi := map[int64]int{} // id kategori -> posisinya di slice, untuk menempelkan tahap
	for rows.Next() {
		var k Kategori
		if err := rows.Scan(&k.ID, &k.EventID, &k.Nama, &k.Kuota); err != nil {
			return nil, err
		}
		posisi[k.ID] = len(kategori)
		kategori = append(kategori, k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. ambil semua tahap milik kategori-kategori tersebut
	rowsTahap, err := q.QueryContext(ctx,
		`SELECT t.id, t.category_id, t.nama, t.urutan, t.harga, t.kuota, t.sisa_kuota, t.batas_tanggal
		FROM price_tiers t
		JOIN ticket_categories c ON c.id = t.category_id
		WHERE c.event_id = ? AND c.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY t.urutan`,
		eventID)
	if err != nil {
		return nil, err
	}
	defer rowsTahap.Close()

	for rowsTahap.Next() {
		var t Tahap
		if err := rowsTahap.Scan(&t.ID, &t.KategoriID, &t.Nama, &t.Urutan, &t.Harga, &t.Kuota, &t.SisaKuota, &t.BatasTanggal); err != nil {
			return nil, err
		}
		i := posisi[t.KategoriID]
		kategori[i].Tahap = append(kategori[i].Tahap, t)
	}
	return kategori, rowsTahap.Err()
}

func simpanEvent(ctx context.Context, db *sql.DB, e Event) (int64, error) {
	// event baru selalu DRAFT (nilai default kolom status)
	res, err := db.ExecContext(ctx,
		"INSERT INTO events (nama, lokasi, deskripsi, tanggal_event, kapasitas) VALUES (?, ?, ?, ?, ?)",
		e.Nama, e.Lokasi, e.Deskripsi, e.TanggalEvent, e.Kapasitas,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ===== Ubah data (dalam transaksi) =====
//
// Semua perubahan yang menyangkut kuota memakai pola yang sama:
//   1. BeginTx
//   2. kunci baris event dengan SELECT ... FOR UPDATE (kunciEventDraft)
//   3. hitung total kuota, cek aturan BR
//   4. simpan, lalu Commit
// Selama baris event terkunci, request lain yang ingin mengubah event yang sama
// harus menunggu, sehingga dua admin tidak bisa sama-sama lolos pengecekan kuota.

// kunciEventDraft mengunci baris event sampai transaksi selesai,
// dan memastikan event masih DRAFT (struktur kuota event OPEN tidak boleh diubah).
func kunciEventDraft(ctx context.Context, tx *sql.Tx, id int64) (Event, error) {
	e, err := KunciEvent(ctx, tx, id)
	if err != nil {
		return e, err
	}
	if e.Status != StatusDraft {
		return e, ErrBukanDraft
	}
	return e, nil
}

// totalKuotaKategori menjumlahkan kuota semua kategori sebuah event
func totalKuotaKategori(ctx context.Context, tx *sql.Tx, eventID int64) (int, error) {
	var total int
	err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(kuota), 0) FROM ticket_categories WHERE event_id = ? AND deleted_at IS NULL",
		eventID,
	).Scan(&total)
	return total, err
}

func updateEvent(ctx context.Context, db *sql.DB, e Event) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() // dibatalkan jika belum Commit; tidak berpengaruh setelah Commit

	if _, err := kunciEventDraft(ctx, tx, e.ID); err != nil {
		return err
	}

	// BR-02: kapasitas baru tidak boleh lebih kecil dari total kuota kategori
	total, err := totalKuotaKategori(ctx, tx, e.ID)
	if err != nil {
		return err
	}
	if err := CekKapasitasBaru(e.Kapasitas, total); err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		"UPDATE events SET nama=?, lokasi=?, deskripsi=?, tanggal_event=?, kapasitas=?, updated_at=NOW() WHERE id=?",
		e.Nama, e.Lokasi, e.Deskripsi, e.TanggalEvent, e.Kapasitas, e.ID,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// hapusEvent melakukan soft delete event beserta kategori dan tahapnya (hanya DRAFT)
func hapusEvent(ctx context.Context, db *sql.DB, id int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := kunciEventDraft(ctx, tx, id); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE price_tiers SET deleted_at = NOW()
		WHERE deleted_at IS NULL AND category_id IN (SELECT id FROM ticket_categories WHERE event_id = ?)`, id,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE ticket_categories SET deleted_at = NOW() WHERE event_id = ? AND deleted_at IS NULL", id,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE events SET deleted_at = NOW() WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func tambahKategori(ctx context.Context, db *sql.DB, eventID int64, nama string, kuota int) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	e, err := kunciEventDraft(ctx, tx, eventID)
	if err != nil {
		return 0, err
	}

	// BR-02: total kuota kategori (yang sudah ada + yang baru) tidak boleh melebihi kapasitas
	total, err := totalKuotaKategori(ctx, tx, eventID)
	if err != nil {
		return 0, err
	}
	if err := CekKuotaKategori(e.Kapasitas, total, kuota); err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx,
		"INSERT INTO ticket_categories (event_id, nama, kuota) VALUES (?, ?, ?)",
		eventID, nama, kuota,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// hapusKategori melakukan soft delete kategori beserta tahapnya
func hapusKategori(ctx context.Context, db *sql.DB, eventID, kategoriID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := kunciEventDraft(ctx, tx, eventID); err != nil {
		return err
	}

	// event_id ikut dicek supaya kategori event lain tidak bisa terhapus lewat URL yang diubah
	res, err := tx.ExecContext(ctx,
		"UPDATE ticket_categories SET deleted_at = NOW() WHERE id = ? AND event_id = ? AND deleted_at IS NULL",
		kategoriID, eventID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTidakDitemukan
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE price_tiers SET deleted_at = NOW() WHERE category_id = ? AND deleted_at IS NULL", kategoriID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func tambahTahap(ctx context.Context, db *sql.DB, eventID, kategoriID int64, t Tahap) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// baris event yang dikunci juga melindungi kategori dan tahapnya,
	// karena semua perubahan kuota event ini selalu mengunci baris event lebih dulu
	if _, err := kunciEventDraft(ctx, tx, eventID); err != nil {
		return 0, err
	}

	var kuotaKategori int
	err = tx.QueryRowContext(ctx,
		"SELECT kuota FROM ticket_categories WHERE id = ? AND event_id = ? AND deleted_at IS NULL",
		kategoriID, eventID,
	).Scan(&kuotaKategori)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTidakDitemukan
	}
	if err != nil {
		return 0, err
	}

	// BR-04: total kuota tahap (yang sudah ada + yang baru) tidak boleh melebihi kuota kategori
	var totalTahap int
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(kuota), 0) FROM price_tiers WHERE category_id = ? AND deleted_at IS NULL",
		kategoriID,
	).Scan(&totalTahap); err != nil {
		return 0, err
	}
	if err := CekKuotaTahap(kuotaKategori, totalTahap, t.Kuota); err != nil {
		return 0, err
	}

	// BR-03: urutan tahap = urutan terakhir + 1.
	// Tahap yang sudah dihapus ikut dihitung, supaya nomor urutan tidak bentrok dengan UNIQUE.
	var urutan int
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(urutan), 0) + 1 FROM price_tiers WHERE category_id = ?", kategoriID,
	).Scan(&urutan); err != nil {
		return 0, err
	}

	// sisa_kuota awal sama dengan kuota; berkurang saat tiket dipesan (feat/order)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO price_tiers (category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		kategoriID, t.Nama, urutan, t.Harga, t.Kuota, t.Kuota, t.BatasTanggal,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func hapusTahap(ctx context.Context, db *sql.DB, eventID, tahapID int64) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := kunciEventDraft(ctx, tx, eventID); err != nil {
		return err
	}

	// tahap hanya terhapus jika memang milik kategori dari event ini
	res, err := tx.ExecContext(ctx,
		`UPDATE price_tiers SET deleted_at = NOW()
		WHERE id = ? AND deleted_at IS NULL
		AND category_id IN (SELECT id FROM ticket_categories WHERE event_id = ? AND deleted_at IS NULL)`,
		tahapID, eventID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrTidakDitemukan
	}
	return tx.Commit()
}

// bukaPenjualan mengubah event dari DRAFT menjadi OPEN jika semua syarat terpenuhi.
// Jika ada syarat yang gagal, daftar masalah dikembalikan dan status tetap DRAFT.
func bukaPenjualan(ctx context.Context, db *sql.DB, eventID int64, now time.Time) ([]string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// kunci event: selama pengecekan berlangsung, tidak ada yang bisa
	// menambah/menghapus kategori atau tahap event ini
	e, err := kunciEventDraft(ctx, tx, eventID)
	if err != nil {
		return nil, err
	}

	// data dibaca lewat tx, jadi yang dicek adalah keadaan yang sedang terkunci
	kategori, err := AmbilKategoriDanTahap(ctx, tx, eventID)
	if err != nil {
		return nil, err
	}

	if masalah := ValidasiBukaPenjualan(e, kategori, now); len(masalah) > 0 {
		return masalah, nil // Rollback lewat defer, status tetap DRAFT
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE events SET status = ?, updated_at = NOW() WHERE id = ?", StatusOpen, eventID,
	); err != nil {
		return nil, err
	}
	return nil, tx.Commit()
}
