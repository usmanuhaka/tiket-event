package checkin

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// Helper bersama untuk test integrasi paket checkin. Data dibuat langsung dengan SQL.

func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.KosongkanSemua(t, db)
	return db
}

// tanggalEvent tetap: 20 Des 2026 19:00 WIB; waktu "di pintu" diberikan lewat parameter now
var tanggalEvent = time.Date(2026, time.December, 20, 19, 0, 0, 0, web.Jakarta)

// diPintu adalah jam di hari event saat petugas memeriksa tiket
func diPintu(jam, menit int) time.Time {
	return time.Date(2026, time.December, 20, jam, menit, 0, 0, web.Jakarta)
}

var nomor int

func buatUser(t *testing.T, db *sql.DB, role string) int64 {
	t.Helper()
	nomor++
	res, err := db.Exec("INSERT INTO users (nama, email, password_hash, role) VALUES (?, ?, 'x', ?)",
		fmt.Sprintf("%s %d", role, nomor), fmt.Sprintf("user%d@kampus.test", nomor), role)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// tiketUji adalah satu tiket hasil siapkanTiket
type tiketUji struct {
	id   int64
	kode string
}

// siapkanTiket membuat event (status OPEN) pada tanggal tertentu dengan satu pesanan PAID berisi n tiket ACTIVE.
// Mengembalikan id event dan tiket-tiketnya.
func siapkanTiket(t *testing.T, db *sql.DB, namaEvent string, tanggal time.Time, n int) (int64, []tiketUji) {
	t.Helper()
	res, err := db.Exec(
		"INSERT INTO events (nama, lokasi, tanggal_event, kapasitas, status) VALUES (?, 'Aula', ?, 100, 'OPEN')",
		namaEvent, tanggal)
	if err != nil {
		t.Fatal(err)
	}
	eventID, _ := res.LastInsertId()
	res, _ = db.Exec("INSERT INTO ticket_categories (event_id, nama, kuota) VALUES (?, 'VIP', 100)", eventID)
	kategoriID, _ := res.LastInsertId()
	res, _ = db.Exec(
		`INSERT INTO price_tiers (category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal)
		VALUES (?, 'Normal', 1, 100000, 100, ?, ?)`, kategoriID, 100-n, tanggal)
	tahapID, _ := res.LastInsertId()

	pembeli := buatUser(t, db, "PEMBELI")
	res, err = db.Exec(
		`INSERT INTO orders (user_id, event_id, status, total_harga, total_bayar, expires_at, paid_at)
		VALUES (?, ?, 'PAID', ?, ?, ?, ?)`, pembeli, eventID, 100000*n, 100000*n, time.Now(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	orderID, _ := res.LastInsertId()

	var tiket []tiketUji
	for i := 0; i < n; i++ {
		kode := fmt.Sprintf("TKT-%04d%04d", orderID%10000, i)
		res, err := db.Exec(
			`INSERT INTO tickets (order_id, category_id, price_tier_id, nama_pemegang, harga, harga_bayar, kode_tiket, status)
			VALUES (?, ?, ?, ?, 100000, 100000, ?, 'ACTIVE')`,
			orderID, kategoriID, tahapID, fmt.Sprintf("Pemegang %d", i+1), kode)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		tiket = append(tiket, tiketUji{id: id, kode: kode})
	}
	return eventID, tiket
}

func hitung(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func statusTiket(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	var s string
	if err := db.QueryRow("SELECT status FROM tickets WHERE id = ?", id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}
