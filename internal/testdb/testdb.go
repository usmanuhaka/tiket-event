// Package testdb membantu test yang butuh database sungguhan (test integrasi).
package testdb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// Buka membuka koneksi ke database test (TEST_DB_DSN).
// Jika TEST_DB_DSN kosong, test di-skip supaya `go test ./...` tetap jalan tanpa database.
// Koneksi ditutup otomatis saat test selesai.
func Buka(t *testing.T) *sql.DB {
	t.Helper()
	muatEnv()

	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN kosong, test database di-skip")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("buka database test: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("database test tidak bisa dihubungi: %v", err)
	}
	return db
}

// Kosongkan menghapus seluruh isi tabel yang disebut, sesuai urutan.
// Tulis tabel anak lebih dulu (mis. "sessions", "users") supaya foreign key tidak menolak.
func Kosongkan(t *testing.T, db *sql.DB, tabel ...string) {
	t.Helper()
	for _, nama := range tabel {
		// nama tabel tidak bisa memakai placeholder ?, dan nilainya hanya dari kode test
		if _, err := db.Exec("DELETE FROM " + nama); err != nil {
			t.Fatalf("kosongkan tabel %s: %v", nama, err)
		}
	}
}

// semuaTabel berisi semua tabel aplikasi, diurutkan dari tabel anak ke tabel induk.
// Tambahkan tabel baru di sini setiap ada migrasi baru.
var semuaTabel = []string{
	"refunds", "ticket_transfers",
	"tickets", "orders",
	"promo_categories", "promo_codes",
	"price_tiers", "ticket_categories", "events",
	"sessions", "users",
}

// KosongkanSemua menghapus isi semua tabel. Dipakai di awal setiap test integrasi,
// supaya data sisa test paket lain (yang memakai database test yang sama) tidak mengganggu.
func KosongkanSemua(t *testing.T, db *sql.DB) {
	t.Helper()
	Kosongkan(t, db, semuaTabel...)
}

// muatEnv membaca .env di root proyek (folder yang berisi go.mod).
// go test menjalankan test dari folder paket, jadi .env harus dicari ke atas.
// Variabel yang sudah di-set (mis. di CI) tidak ditimpa oleh godotenv.
func muatEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			_ = godotenv.Load(filepath.Join(dir, ".env"))
			return
		}
		induk := filepath.Dir(dir)
		if induk == dir {
			return
		}
		dir = induk
	}
}
