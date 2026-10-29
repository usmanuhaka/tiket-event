package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
)

// bukaDBKosong membuka database test dengan tabel users dan sessions kosong
func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.Kosongkan(t, db, "sessions", "users")
	return db
}

// buatUser menyimpan user contoh dan mengembalikan id-nya
func buatUser(t *testing.T, db *sql.DB, email, password, role string) int64 {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	id, err := simpanUser(context.Background(), db, User{Nama: "Contoh", Email: email, PasswordHash: hash, Role: role})
	if err != nil {
		t.Fatalf("simpan user: %v", err)
	}
	return id
}

func TestSimpanDanAmbilUser(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()

	id := buatUser(t, db, "budi@kampus.test", "rahasia123", PeranPembeli)

	u, err := getUserByEmail(ctx, db, "budi@kampus.test")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != id || u.Role != PeranPembeli {
		t.Errorf("dapat %+v", u)
	}
	if !CekPassword(u.PasswordHash, "rahasia123") {
		t.Error("hash di database tidak cocok dengan password")
	}
}

func TestEmailGandaDitolak(t *testing.T) {
	db := bukaDBKosong(t)

	buatUser(t, db, "budi@kampus.test", "rahasia123", PeranPembeli)

	_, err := simpanUser(context.Background(), db, User{Nama: "Budi 2", Email: "budi@kampus.test", PasswordHash: "x", Role: PeranPembeli})
	if !errors.Is(err, ErrEmailTerdaftar) {
		t.Errorf("mau ErrEmailTerdaftar, dapat %v", err)
	}
}

func TestSesiAktifKedaluwarsaDanLogout(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	id := buatUser(t, db, "budi@kampus.test", "rahasia123", PeranPembeli)

	// pakai waktu tanpa detik pecahan supaya sama persis dengan kolom DATETIME
	now := time.Now().Truncate(time.Second)
	token := BuatToken()
	if err := simpanSession(ctx, db, token, id, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// sesi masih berlaku
	u, err := getUserBySessionToken(ctx, db, token, now)
	if err != nil || u.ID != id {
		t.Fatalf("sesi aktif tidak ditemukan: %v", err)
	}

	// dua jam kemudian, sesi sudah kedaluwarsa
	_, err = getUserBySessionToken(ctx, db, token, now.Add(2*time.Hour))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("sesi kedaluwarsa masih ditemukan: %v", err)
	}

	// setelah logout, token tidak berlaku lagi
	if err := hapusSession(ctx, db, token); err != nil {
		t.Fatal(err)
	}
	_, err = getUserBySessionToken(ctx, db, token, now)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("sesi masih ada setelah logout: %v", err)
	}
}

func TestUserTerhapusTidakBisaLogin(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	id := buatUser(t, db, "budi@kampus.test", "rahasia123", PeranPembeli)

	now := time.Now().Truncate(time.Second)
	token := BuatToken()
	if err := simpanSession(ctx, db, token, id, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	// soft delete user
	if _, err := db.ExecContext(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}

	if _, err := getUserByEmail(ctx, db, "budi@kampus.test"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("user terhapus masih ditemukan lewat email: %v", err)
	}
	if _, err := getUserBySessionToken(ctx, db, token, now); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("sesi user terhapus masih berlaku: %v", err)
	}
}
