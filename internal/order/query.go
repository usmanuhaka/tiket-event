package order

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/event"
)

// ErrTidakDitemukan dikembalikan jika event atau pesanan tidak ada,
// atau pesanan bukan milik pembeli yang sedang login
var ErrTidakDitemukan = errors.New("Data tidak ditemukan")

// Pesanan adalah satu baris tabel orders beserta tiketnya
type Pesanan struct {
	ID            int64
	UserID        int64
	EventID       int64
	NamaEvent     string
	TanggalEvent  time.Time
	Status        string
	TotalHarga    int64
	TotalPotongan int64
	TotalBayar    int64
	ExpiresAt     time.Time
	PaidAt        sql.NullTime
	CreatedAt     time.Time
	Tiket         []Tiket
}

// Tiket adalah satu baris tabel tickets untuk ditampilkan
type Tiket struct {
	ID           int64
	NamaKategori string
	NamaTahap    string
	NamaPemegang string
	Harga        int64
	Potongan     int64
	HargaBayar   int64
	KodeTiket    string // kosong selama HELD
	Status       string
}

// ===== Buat pesanan (BR-06 s.d. BR-09, BR-12) =====

// buatPesanan menahan tiket untuk pembeli: kuota langsung berkurang dan pesanan berstatus HELD
// selama 15 menit. Semua langkah ada di satu transaksi, jadi jika satu langkah gagal
// tidak ada kuota yang berkurang dan tidak ada pesanan setengah jadi.
func buatPesanan(ctx context.Context, db *sql.DB, userID, eventID int64, baris []BarisTiket, now time.Time) (int64, error) {
	if err := ValidasiPesanan(baris); err != nil {
		return 0, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // dibatalkan jika belum Commit

	// 1. kunci baris event: pemesanan untuk event yang sama dilayani satu per satu
	e, err := event.KunciEvent(ctx, tx, eventID)
	if errors.Is(err, event.ErrTidakDitemukan) {
		return 0, ErrTidakDitemukan
	}
	if err != nil {
		return 0, err
	}
	if e.Status != event.StatusOpen {
		return 0, salahInput("Event ini tidak sedang dibuka untuk pemesanan")
	}
	if !e.TanggalEvent.After(now) {
		return 0, salahInput("Event sudah berlangsung, tiket tidak bisa dipesan lagi")
	}

	// BR-06: semua kategori harus milik event ini
	namaKategori, err := kategoriEvent(ctx, tx, eventID)
	if err != nil {
		return 0, err
	}
	var kategoriDipesan []int64 // urutan kemunculan di form, tanpa duplikat
	jumlahPerKategori := map[int64]int{}
	for _, b := range baris {
		if _, ada := namaKategori[b.KategoriID]; !ada {
			return 0, salahInput("Kategori tiket tidak valid untuk event ini")
		}
		if jumlahPerKategori[b.KategoriID] == 0 {
			kategoriDipesan = append(kategoriDipesan, b.KategoriID)
		}
		jumlahPerKategori[b.KategoriID]++
	}

	// 2. kunci semua tahap kategori yang dipesan, berurutan menurut id
	semuaTahap, err := event.KunciTahapKategori(ctx, tx, kategoriDipesan)
	if err != nil {
		return 0, err
	}
	tahapPerKategori := map[int64][]event.Tahap{}
	for _, t := range semuaTahap {
		tahapPerKategori[t.KategoriID] = append(tahapPerKategori[t.KategoriID], t)
	}

	// BR-08: tentukan harga tiap tiket dengan alokasi berurutan, per kategori
	alokasiKategori := map[int64][]Alokasi{}
	for _, k := range kategoriDipesan {
		alokasi, err := AlokasiTiket(tahapPerKategori[k], jumlahPerKategori[k], now)
		if err != nil {
			return 0, salahInput("Kategori %s: %s", namaKategori[k], err.Error())
		}
		alokasiKategori[k] = alokasi
	}

	// pasangkan hasil alokasi ke baris form: tiket pertama tiap kategori mendapat harga termurah
	tiket := make([]Alokasi, len(baris))
	dipakai := map[int64]int{}
	var totalHarga int64
	for i, b := range baris {
		tiket[i] = alokasiKategori[b.KategoriID][dipakai[b.KategoriID]]
		dipakai[b.KategoriID]++
		totalHarga += tiket[i].Harga
	}

	// BR-09: kuota langsung berkurang saat dipesan
	ambilPerTahap := map[int64]int{}
	for _, a := range tiket {
		ambilPerTahap[a.TahapID]++
	}
	idTahap := make([]int64, 0, len(ambilPerTahap))
	for id := range ambilPerTahap {
		idTahap = append(idTahap, id)
	}
	sort.Slice(idTahap, func(i, j int) bool { return idTahap[i] < idTahap[j] })
	for _, id := range idTahap {
		if _, err := tx.ExecContext(ctx,
			"UPDATE price_tiers SET sisa_kuota = sisa_kuota - ?, updated_at = NOW() WHERE id = ?",
			ambilPerTahap[id], id,
		); err != nil {
			return 0, err
		}
	}

	// BR-09: pesanan HELD, harus dibayar paling lambat 15 menit lagi
	// (promo belum ada: potongan 0, total bayar = total harga)
	res, err := tx.ExecContext(ctx,
		`INSERT INTO orders (user_id, event_id, status, total_harga, total_potongan, total_bayar, expires_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)`,
		userID, eventID, StatusHeld, totalHarga, totalHarga, BatasBayar(now),
	)
	if err != nil {
		return 0, err
	}
	orderID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// BR-07: satu baris tiket per pemegang, kode tiket baru dibuat saat dibayar
	for i, b := range baris {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO tickets (order_id, category_id, price_tier_id, nama_pemegang, harga, potongan, harga_bayar, status)
			VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
			orderID, b.KategoriID, tiket[i].TahapID, strings.TrimSpace(b.NamaPemegang), tiket[i].Harga, tiket[i].Harga, TiketHeld,
		); err != nil {
			return 0, err
		}
	}

	return orderID, tx.Commit()
}

// kategoriEvent mengembalikan id -> nama semua kategori sebuah event
func kategoriEvent(ctx context.Context, tx *sql.Tx, eventID int64) (map[int64]string, error) {
	rows, err := tx.QueryContext(ctx,
		"SELECT id, nama FROM ticket_categories WHERE event_id = ? AND deleted_at IS NULL", eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hasil := map[int64]string{}
	for rows.Next() {
		var id int64
		var nama string
		if err := rows.Scan(&id, &nama); err != nil {
			return nil, err
		}
		hasil[id] = nama
	}
	return hasil, rows.Err()
}

// ===== Baca pesanan =====

// ambilPesananMilik mengambil satu pesanan beserta tiketnya, hanya jika milik userID.
// Pesanan orang lain dianggap tidak ada, supaya keberadaannya tidak bocor.
func ambilPesananMilik(ctx context.Context, db *sql.DB, id, userID int64) (Pesanan, error) {
	var p Pesanan

	err := db.QueryRowContext(ctx,
		`SELECT o.id, o.user_id, o.event_id, e.nama, e.tanggal_event, o.status,
			o.total_harga, o.total_potongan, o.total_bayar, o.expires_at, o.paid_at, o.created_at
		FROM orders o JOIN events e ON e.id = o.event_id
		WHERE o.id = ? AND o.user_id = ?`, id, userID,
	).Scan(&p.ID, &p.UserID, &p.EventID, &p.NamaEvent, &p.TanggalEvent, &p.Status,
		&p.TotalHarga, &p.TotalPotongan, &p.TotalBayar, &p.ExpiresAt, &p.PaidAt, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrTidakDitemukan
	}
	if err != nil {
		return p, err
	}

	rows, err := db.QueryContext(ctx,
		`SELECT t.id, c.nama, pt.nama, t.nama_pemegang, t.harga, t.potongan, t.harga_bayar,
			COALESCE(t.kode_tiket, ''), t.status
		FROM tickets t
		JOIN ticket_categories c ON c.id = t.category_id
		JOIN price_tiers pt ON pt.id = t.price_tier_id
		WHERE t.order_id = ?
		ORDER BY t.id`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()

	for rows.Next() {
		var t Tiket
		if err := rows.Scan(&t.ID, &t.NamaKategori, &t.NamaTahap, &t.NamaPemegang,
			&t.Harga, &t.Potongan, &t.HargaBayar, &t.KodeTiket, &t.Status); err != nil {
			return p, err
		}
		p.Tiket = append(p.Tiket, t)
	}
	return p, rows.Err()
}

// ambilPesananUser mengambil semua pesanan seorang pembeli (tanpa daftar tiket), terbaru di atas
func ambilPesananUser(ctx context.Context, db *sql.DB, userID int64) ([]Pesanan, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT o.id, o.user_id, o.event_id, e.nama, e.tanggal_event, o.status,
			o.total_harga, o.total_potongan, o.total_bayar, o.expires_at, o.paid_at, o.created_at
		FROM orders o JOIN events e ON e.id = o.event_id
		WHERE o.user_id = ?
		ORDER BY o.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hasil []Pesanan
	for rows.Next() {
		var p Pesanan
		if err := rows.Scan(&p.ID, &p.UserID, &p.EventID, &p.NamaEvent, &p.TanggalEvent, &p.Status,
			&p.TotalHarga, &p.TotalPotongan, &p.TotalBayar, &p.ExpiresAt, &p.PaidAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		hasil = append(hasil, p)
	}
	return hasil, rows.Err()
}
