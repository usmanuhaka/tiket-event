package event

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// File ini berisi fungsi bersama untuk paket lain (order, ticket) yang mengubah kuota tahap.
//
// Urutan kunci yang WAJIB diikuti setiap transaksi yang mengubah kuota:
//   1. baris event         -> KunciEvent
//   2. baris pesanan       -> (di paket order, jika ada)
//   3. baris tahap harga   -> KunciTahapKategori (selalu ORDER BY id)
// Karena semua transaksi mengunci dengan urutan yang sama, dua transaksi tidak bisa
// saling menunggu (deadlock), dan transaksi untuk event yang sama dilayani satu per satu.

// KunciEvent mengunci baris event sampai transaksi selesai (SELECT ... FOR UPDATE).
// Berbeda dengan kunciEventDraft, fungsi ini tidak memeriksa status;
// pemanggil yang memutuskan status mana yang boleh.
func KunciEvent(ctx context.Context, tx *sql.Tx, id int64) (Event, error) {
	var e Event

	err := tx.QueryRowContext(ctx,
		`SELECT id, nama, lokasi, COALESCE(deskripsi, ''), tanggal_event, kapasitas, status
		FROM events WHERE id = ? AND deleted_at IS NULL
		FOR UPDATE`, id,
	).Scan(&e.ID, &e.Nama, &e.Lokasi, &e.Deskripsi, &e.TanggalEvent, &e.Kapasitas, &e.Status)

	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrTidakDitemukan
	}
	return e, err
}

// KunciTahapKategori mengunci semua tahap dari kategori-kategori yang disebut,
// berurutan menurut id, lalu mengembalikannya. Tahap berkuota 0 ikut dikunci dan dikembalikan.
func KunciTahapKategori(ctx context.Context, tx *sql.Tx, kategoriID []int64) ([]Tahap, error) {
	if len(kategoriID) == 0 {
		return nil, nil
	}

	// membuat "?, ?, ?" sebanyak jumlah kategori untuk IN (...)
	tanda := strings.TrimSuffix(strings.Repeat("?, ", len(kategoriID)), ", ")
	args := make([]any, len(kategoriID))
	for i, id := range kategoriID {
		args[i] = id
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal
		FROM price_tiers
		WHERE category_id IN (`+tanda+`) AND deleted_at IS NULL
		ORDER BY id
		FOR UPDATE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []Tahap
	for rows.Next() {
		var t Tahap
		if err := rows.Scan(&t.ID, &t.KategoriID, &t.Nama, &t.Urutan, &t.Harga, &t.Kuota, &t.SisaKuota, &t.BatasTanggal); err != nil {
			return nil, err
		}
		hasil = append(hasil, t)
	}
	return hasil, rows.Err()
}

// KembalikanKuota mengembalikan 1 kuota tiket ke tahapnya, di dalam transaksi tx.
// Dipakai saat pesanan kedaluwarsa (BR-10) dan saat tiket dibatalkan pembeli (BR-19).
// Pemanggil WAJIB sudah mengunci baris event dan tahap kategori tersebut (KunciEvent, KunciTahapKategori).
// Mengembalikan id tahap yang menerima kuota.
func KembalikanKuota(ctx context.Context, tx *sql.Tx, kategoriID, tahapAsalID int64, now time.Time) (int64, error) {
	// baca keadaan tahap terbaru di dalam transaksi (sudah terkunci oleh pemanggil)
	tahap, err := tahapKategori(ctx, tx, kategoriID)
	if err != nil {
		return 0, err
	}
	tujuan := TahapTujuanPengembalian(tahap, tahapAsalID, now)

	if tujuan == tahapAsalID {
		// tahap asal masih berlaku (atau tidak ada tahap lain yang berlaku): sisa kuota +1
		_, err = tx.ExecContext(ctx,
			"UPDATE price_tiers SET sisa_kuota = sisa_kuota + 1, updated_at = NOW() WHERE id = ?", tahapAsalID)
		return tahapAsalID, err
	}

	// tahap asal sudah lewat: pindahkan 1 kuota ke tahap tujuan.
	// kuota asal -1 dan kuota tujuan +1, jadi jumlah kuota tahap tetap sama dengan kuota kategori (BR-04).
	if _, err := tx.ExecContext(ctx,
		"UPDATE price_tiers SET kuota = kuota - 1, updated_at = NOW() WHERE id = ?", tahapAsalID,
	); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE price_tiers SET kuota = kuota + 1, sisa_kuota = sisa_kuota + 1, updated_at = NOW() WHERE id = ?", tujuan)
	return tujuan, err
}

// tahapKategori membaca semua tahap satu kategori di dalam transaksi
func tahapKategori(ctx context.Context, tx *sql.Tx, kategoriID int64) ([]Tahap, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal
		FROM price_tiers WHERE category_id = ? AND deleted_at IS NULL ORDER BY urutan`, kategoriID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []Tahap
	for rows.Next() {
		var t Tahap
		if err := rows.Scan(&t.ID, &t.KategoriID, &t.Nama, &t.Urutan, &t.Harga, &t.Kuota, &t.SisaKuota, &t.BatasTanggal); err != nil {
			return nil, err
		}
		hasil = append(hasil, t)
	}
	return hasil, rows.Err()
}
