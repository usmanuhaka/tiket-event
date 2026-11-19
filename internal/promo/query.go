package promo

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// ErrTidakDitemukan dikembalikan jika kode promo tidak ada atau sudah dihapus
var ErrTidakDitemukan = KesalahanInput{Pesan: "Kode promo tidak ditemukan"}

// ErrKodeTerdaftar dikembalikan jika kode sudah dipakai promo lain (termasuk yang sudah dihapus)
var ErrKodeTerdaftar = KesalahanInput{Pesan: "Kode promo sudah dipakai, gunakan kode lain"}

// kodeDuplikat adalah nomor error MySQL/MariaDB untuk pelanggaran UNIQUE
const kodeDuplikat = 1062

// PilihanKategori adalah satu kotak centang kategori di form promo
type PilihanKategori struct {
	ID        int64
	NamaEvent string
	Nama      string
}

// ===== Halaman admin =====

// ambilSemuaPromo mengambil promo yang belum dihapus beserta nama kategorinya
func ambilSemuaPromo(ctx context.Context, db *sql.DB) ([]Promo, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, kode, persen, kuota_pemakaian, terpakai
		FROM promo_codes WHERE deleted_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []Promo
	posisi := map[int64]int{} // id promo -> posisinya di slice
	for rows.Next() {
		var p Promo
		if err := rows.Scan(&p.ID, &p.Kode, &p.Persen, &p.KuotaPemakaian, &p.Terpakai); err != nil {
			return nil, err
		}
		posisi[p.ID] = len(hasil)
		hasil = append(hasil, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rowsKategori, err := db.QueryContext(ctx,
		`SELECT pc.promo_code_id, c.id, CONCAT(e.nama, ' - ', c.nama)
		FROM promo_categories pc
		JOIN ticket_categories c ON c.id = pc.category_id
		JOIN events e ON e.id = c.event_id
		JOIN promo_codes p ON p.id = pc.promo_code_id
		WHERE p.deleted_at IS NULL
		ORDER BY e.tanggal_event, c.id`)
	if err != nil {
		return nil, err
	}
	defer rowsKategori.Close()

	for rowsKategori.Next() {
		var promoID, kategoriID int64
		var nama string
		if err := rowsKategori.Scan(&promoID, &kategoriID, &nama); err != nil {
			return nil, err
		}
		i := posisi[promoID]
		hasil[i].KategoriID = append(hasil[i].KategoriID, kategoriID)
		hasil[i].NamaKategori = append(hasil[i].NamaKategori, nama)
	}
	return hasil, rowsKategori.Err()
}

// ambilPromo mengambil promo yang belum dihapus beserta kategori yang dipilih.
func ambilPromo(ctx context.Context, db *sql.DB, id int64) (Promo, error) {
	var p Promo
	err := db.QueryRowContext(ctx,
		`SELECT id, kode, persen, kuota_pemakaian, terpakai
		FROM promo_codes WHERE id = ? AND deleted_at IS NULL`, id,
	).Scan(&p.ID, &p.Kode, &p.Persen, &p.KuotaPemakaian, &p.Terpakai)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrTidakDitemukan
	}
	if err != nil {
		return p, err
	}

	rows, err := db.QueryContext(ctx,
		"SELECT category_id FROM promo_categories WHERE promo_code_id = ? ORDER BY category_id", p.ID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var kategoriID int64
		if err := rows.Scan(&kategoriID); err != nil {
			return p, err
		}
		p.KategoriID = append(p.KategoriID, kategoriID)
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	return p, nil
}

// ambilPilihanKategori mengambil kategori yang bisa dipilih di form promo:
// kategori dari event DRAFT dan OPEN yang belum dihapus
func ambilPilihanKategori(ctx context.Context, db *sql.DB) ([]PilihanKategori, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT c.id, e.nama, c.nama
		FROM ticket_categories c
		JOIN events e ON e.id = c.event_id
		WHERE c.deleted_at IS NULL AND e.deleted_at IS NULL AND e.status IN ('DRAFT', 'OPEN')
		ORDER BY e.tanggal_event, c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []PilihanKategori
	for rows.Next() {
		var k PilihanKategori
		if err := rows.Scan(&k.ID, &k.NamaEvent, &k.Nama); err != nil {
			return nil, err
		}
		hasil = append(hasil, k)
	}
	return hasil, rows.Err()
}

// simpanPromo menyimpan promo beserta kategorinya dalam satu transaksi,
// supaya tidak ada promo tersimpan tanpa kategori jika salah satu langkah gagal
func simpanPromo(ctx context.Context, db *sql.DB, p Promo) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	unik, err := validasiKategoriPromo(ctx, tx, p.KategoriID)
	if err != nil {
		return 0, err
	}

	res, err := tx.ExecContext(ctx,
		"INSERT INTO promo_codes (kode, persen, kuota_pemakaian) VALUES (?, ?, ?)",
		p.Kode, p.Persen, p.KuotaPemakaian,
	)
	if err != nil {
		var errMysql *mysql.MySQLError
		if errors.As(err, &errMysql) && errMysql.Number == kodeDuplikat {
			return 0, ErrKodeTerdaftar
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, kategoriID := range unik {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO promo_categories (promo_code_id, category_id) VALUES (?, ?)", id, kategoriID,
		); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// ubahPromo memperbarui promo beserta kategorinya dalam satu transaksi.
func ubahPromo(ctx context.Context, db *sql.DB, p Promo) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var terpakai int
	var dihapus sql.NullTime
	err = tx.QueryRowContext(ctx,
		"SELECT terpakai, deleted_at FROM promo_codes WHERE id = ? FOR UPDATE", p.ID,
	).Scan(&terpakai, &dihapus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTidakDitemukan
	}
	if err != nil {
		return err
	}
	if dihapus.Valid {
		return ErrTidakDitemukan
	}
	if err := ValidasiKuotaEdit(p.KuotaPemakaian, terpakai); err != nil {
		return err
	}

	unik, err := validasiKategoriPromo(ctx, tx, p.KategoriID)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		"UPDATE promo_codes SET kode = ?, persen = ?, kuota_pemakaian = ?, updated_at = NOW() WHERE id = ?",
		p.Kode, p.Persen, p.KuotaPemakaian, p.ID,
	)
	if err != nil {
		var errMysql *mysql.MySQLError
		if errors.As(err, &errMysql) && errMysql.Number == kodeDuplikat {
			return ErrKodeTerdaftar
		}
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM promo_categories WHERE promo_code_id = ?", p.ID); err != nil {
		return err
	}
	for _, kategoriID := range unik {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO promo_categories (promo_code_id, category_id) VALUES (?, ?)", p.ID, kategoriID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// validasiKategoriPromo membuang kategori duplikat dan memastikan semuanya aktif
// serta berasal dari event DRAFT atau OPEN.
func validasiKategoriPromo(ctx context.Context, tx *sql.Tx, kategoriID []int64) ([]int64, error) {
	unik := make([]int64, 0, len(kategoriID))
	sudah := make(map[int64]bool, len(kategoriID))
	for _, id := range kategoriID {
		if !sudah[id] {
			sudah[id] = true
			unik = append(unik, id)
		}
	}
	if len(unik) == 0 {
		return nil, salahInput("Pilih minimal 1 kategori tempat promo berlaku")
	}

	var jumlah int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ticket_categories c JOIN events e ON e.id = c.event_id
		WHERE c.id IN (`+tandaTanya(len(unik))+`)
		AND c.deleted_at IS NULL AND e.deleted_at IS NULL AND e.status IN ('DRAFT', 'OPEN')`,
		keArgs(unik)...,
	).Scan(&jumlah)
	if err != nil {
		return nil, err
	}
	if jumlah != len(unik) {
		return nil, salahInput("Ada kategori yang tidak valid atau event-nya sudah dibatalkan")
	}
	return unik, nil
}

// hapusPromo melakukan soft delete. Kode tidak bisa dipakai pesanan baru,
// tetapi pesanan lama yang memakainya tetap tercatat.
func hapusPromo(ctx context.Context, db *sql.DB, id int64) error {
	res, err := db.ExecContext(ctx,
		"UPDATE promo_codes SET deleted_at = NOW() WHERE id = ? AND deleted_at IS NULL", id)
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
	return nil
}

// ===== Fungsi bersama untuk paket order =====
//
// Promo SELALU dikunci paling akhir, sesudah event, pesanan, dan tahap
// (urutan lengkap ada di internal/event/kuota.go).

// KunciPromo mengunci baris promo berdasarkan kodenya (SELECT ... FOR UPDATE) lalu mengembalikan
// promo beserta kategorinya. Promo yang sudah dihapus dianggap tidak ada (ErrTidakDitemukan).
func KunciPromo(ctx context.Context, tx *sql.Tx, kode string) (Promo, error) {
	var p Promo
	err := tx.QueryRowContext(ctx,
		`SELECT id, kode, persen, kuota_pemakaian, terpakai
		FROM promo_codes WHERE kode = ? AND deleted_at IS NULL
		FOR UPDATE`, NormalisasiKode(kode),
	).Scan(&p.ID, &p.Kode, &p.Persen, &p.KuotaPemakaian, &p.Terpakai)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrTidakDitemukan
	}
	if err != nil {
		return p, err
	}

	rows, err := tx.QueryContext(ctx, "SELECT category_id FROM promo_categories WHERE promo_code_id = ?", p.ID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return p, err
		}
		p.KategoriID = append(p.KategoriID, id)
	}
	return p, rows.Err()
}

// PakaiPromo (BR-15) menambah 1 pemakaian. Dipanggil saat pesanan dibuat,
// setelah KunciPromo dan pengecekan MasihAdaKuota.
func PakaiPromo(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx,
		"UPDATE promo_codes SET terpakai = terpakai + 1, updated_at = NOW() WHERE id = ?", id)
	return err
}

// KembalikanPemakaian (BR-15) mengurangi 1 pemakaian saat pesanan EXPIRED.
// Baris dikunci lewat id TANPA menyaring deleted_at: jika admin menghapus promo
// selagi pesanan masih HELD, pemakaiannya tetap dikembalikan dengan benar.
func KembalikanPemakaian(ctx context.Context, tx *sql.Tx, id int64) error {
	var terkunci int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM promo_codes WHERE id = ? FOR UPDATE", id).Scan(&terkunci)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTidakDitemukan
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		"UPDATE promo_codes SET terpakai = terpakai - 1, updated_at = NOW() WHERE id = ? AND terpakai > 0", id)
	return err
}

// tandaTanya membuat "?, ?, ?" sebanyak n untuk IN (...)
func tandaTanya(n int) string {
	return strings.TrimSuffix(strings.Repeat("?, ", n), ", ")
}

func keArgs(id []int64) []any {
	args := make([]any, len(id))
	for i, v := range id {
		args[i] = v
	}
	return args
}
