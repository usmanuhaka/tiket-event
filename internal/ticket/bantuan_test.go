package ticket

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
)

// Helper bersama untuk test integrasi paket ticket.
// Data dibuat langsung dengan SQL karena fungsi pemesanan ada di paket order (tidak diekspor).

func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.KosongkanSemua(t, db)
	return db
}

func hari(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

var nomorUser int

func buatPembeli(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	nomorUser++
	res, err := db.Exec("INSERT INTO users (nama, email, password_hash, role) VALUES (?, ?, 'x', 'PEMBELI')",
		fmt.Sprintf("Pembeli %d", nomorUser), fmt.Sprintf("pembeli%d@kampus.test", nomorUser))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// tahapUji: nama, harga, kuota, dan berapa hari lagi batas tanggalnya (negatif = sudah lewat)
type tahapUji struct {
	nama     string
	harga    int64
	kuota    int
	hariLagi int
}

type eventUji struct {
	eventID    int64
	kategoriID int64
	tahapID    []int64
	harga      []int64
}

// siapkanEvent membuat event OPEN pada tanggal tertentu dengan satu kategori
func siapkanEvent(t *testing.T, db *sql.DB, tanggal time.Time, daftar []tahapUji) eventUji {
	t.Helper()
	total := 0
	for _, d := range daftar {
		total += d.kuota
	}
	res, err := db.Exec(
		"INSERT INTO events (nama, lokasi, tanggal_event, kapasitas, status) VALUES ('Konser Test', 'Aula', ?, ?, 'OPEN')",
		tanggal, total)
	if err != nil {
		t.Fatal(err)
	}
	var e eventUji
	e.eventID, _ = res.LastInsertId()

	res, err = db.Exec("INSERT INTO ticket_categories (event_id, nama, kuota) VALUES (?, 'Festival', ?)", e.eventID, total)
	if err != nil {
		t.Fatal(err)
	}
	e.kategoriID, _ = res.LastInsertId()

	now := time.Now().Truncate(time.Second)
	for i, d := range daftar {
		res, err := db.Exec(
			`INSERT INTO price_tiers (category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			e.kategoriID, d.nama, i+1, d.harga, d.kuota, d.kuota, now.Add(hari(d.hariLagi)))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		e.tahapID = append(e.tahapID, id)
		e.harga = append(e.harga, d.harga)
	}
	return e
}

// pesananUji adalah hasil buatPesanan
type pesananUji struct {
	orderID int64
	tiketID []int64
}

// buatPesanan menyimpan pesanan berstatus `status` (PAID atau HELD) berisi n tiket dari tahap ke-indeksTahap.
// Tiket PAID menjadi ACTIVE dengan kode; kuota tahap dikurangi seperti saat pemesanan sungguhan.
// potongan diterapkan ke setiap tiket.
func buatPesanan(t *testing.T, db *sql.DB, userID int64, e eventUji, status string, n, indeksTahap int, potongan int64, promoID sql.NullInt64) pesananUji {
	t.Helper()
	harga := e.harga[indeksTahap]
	statusTiket := "HELD"
	var paidAt sql.NullTime
	if status == "PAID" {
		statusTiket = "ACTIVE"
		paidAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	res, err := db.Exec(
		`INSERT INTO orders (user_id, event_id, promo_code_id, status, total_harga, total_potongan, total_bayar, expires_at, paid_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, e.eventID, promoID, status, harga*int64(n), potongan*int64(n), (harga-potongan)*int64(n),
		time.Now().Add(15*time.Minute), paidAt)
	if err != nil {
		t.Fatal(err)
	}
	var p pesananUji
	p.orderID, _ = res.LastInsertId()

	for i := 0; i < n; i++ {
		var kode sql.NullString
		if statusTiket == "ACTIVE" {
			kode = sql.NullString{String: fmt.Sprintf("TKT-%d-%d", p.orderID, i), Valid: true}
		}
		res, err := db.Exec(
			`INSERT INTO tickets (order_id, category_id, price_tier_id, nama_pemegang, harga, potongan, harga_bayar, kode_tiket, status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.orderID, e.kategoriID, e.tahapID[indeksTahap], fmt.Sprintf("Pemegang %d", i+1),
			harga, potongan, harga-potongan, kode, statusTiket)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		p.tiketID = append(p.tiketID, id)
	}
	if _, err := db.Exec("UPDATE price_tiers SET sisa_kuota = sisa_kuota - ? WHERE id = ?", n, e.tahapID[indeksTahap]); err != nil {
		t.Fatal(err)
	}
	return p
}

func tanpaPromo() sql.NullInt64 { return sql.NullInt64{} }

func hitung(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func teks(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var s string
	if err := db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

type kuotaSisa struct{ kuota, sisa int }

func bacaTahap(t *testing.T, db *sql.DB, id int64) kuotaSisa {
	t.Helper()
	var k kuotaSisa
	if err := db.QueryRow("SELECT kuota, sisa_kuota FROM price_tiers WHERE id = ?", id).Scan(&k.kuota, &k.sisa); err != nil {
		t.Fatal(err)
	}
	return k
}
