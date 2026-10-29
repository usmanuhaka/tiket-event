package order

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
)

// Helper bersama untuk test integrasi paket order.

// bukaDBKosong membuka database test dengan tabel pesanan, event, dan user kosong
func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.Kosongkan(t, db, "tickets", "orders", "price_tiers", "ticket_categories", "events", "sessions", "users")
	return db
}

func waktuSekarang() time.Time {
	return time.Now().Truncate(time.Second)
}

var nomorUser int

// buatPembeli menyimpan user PEMBELI dan mengembalikan id-nya
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

// eventUji adalah hasil siapkanEvent
type eventUji struct {
	eventID    int64
	kategoriID int64
	tahapID    []int64
}

// siapkanEvent membuat event OPEN 30 hari lagi dengan satu kategori dan tahap-tahap yang diminta.
// Data dimasukkan langsung dengan SQL karena fungsi pengelolaan event ada di paket lain.
func siapkanEvent(t *testing.T, db *sql.DB, daftar []tahapUji) eventUji {
	t.Helper()
	now := waktuSekarang()
	total := 0
	for _, d := range daftar {
		total += d.kuota
	}

	res, err := db.Exec(
		"INSERT INTO events (nama, lokasi, tanggal_event, kapasitas, status) VALUES ('Konser Test', 'Aula', ?, ?, 'OPEN')",
		now.Add(30*24*time.Hour), total)
	if err != nil {
		t.Fatal(err)
	}
	var e eventUji
	e.eventID, _ = res.LastInsertId()
	e.kategoriID = tambahKategoriUji(t, db, e.eventID, "Festival", daftar, &e.tahapID)
	return e
}

// tambahKategoriUji menambah kategori beserta tahapnya ke event, id tahap ditambahkan ke *tahapID
func tambahKategoriUji(t *testing.T, db *sql.DB, eventID int64, nama string, daftar []tahapUji, tahapID *[]int64) int64 {
	t.Helper()
	total := 0
	for _, d := range daftar {
		total += d.kuota
	}
	res, err := db.Exec("INSERT INTO ticket_categories (event_id, nama, kuota) VALUES (?, ?, ?)", eventID, nama, total)
	if err != nil {
		t.Fatal(err)
	}
	kategoriID, _ := res.LastInsertId()

	now := waktuSekarang()
	for i, d := range daftar {
		res, err := db.Exec(
			`INSERT INTO price_tiers (category_id, nama, urutan, harga, kuota, sisa_kuota, batas_tanggal)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			kategoriID, d.nama, i+1, d.harga, d.kuota, d.kuota, now.Add(time.Duration(d.hariLagi)*24*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		*tahapID = append(*tahapID, id)
	}
	return kategoriID
}

// pesan membuat baris tiket sebanyak n untuk satu kategori
func pesan(kategoriID int64, n int) []BarisTiket {
	var baris []BarisTiket
	for i := 0; i < n; i++ {
		baris = append(baris, BarisTiket{KategoriID: kategoriID, NamaPemegang: fmt.Sprintf("Pemegang %d", i+1)})
	}
	return baris
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

func hitung(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
