// Package auth mengurus pendaftaran, login, logout, sesi, dan pembatasan halaman per peran.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// Peran pengguna, sama dengan nilai ENUM kolom users.role
const (
	PeranAdmin   = "ADMIN"
	PeranPembeli = "PEMBELI"
	PeranPetugas = "PETUGAS"
)

// LamaSesi adalah berapa lama login berlaku sebelum harus login ulang
const LamaSesi = 24 * time.Hour

// DataDaftar adalah isi form pendaftaran
type DataDaftar struct {
	Nama       string
	Email      string
	Password   string
	Konfirmasi string
}

// NormalisasiEmail membuang spasi di ujung dan membuat huruf kecil,
// supaya "Budi@Kampus.test " dan "budi@kampus.test" dianggap email yang sama.
func NormalisasiEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ValidasiDaftar memeriksa isi form pendaftaran.
// Mengembalikan error berisi pesan untuk pengguna, atau nil jika semua valid.
func ValidasiDaftar(d DataDaftar) error {
	nama := strings.TrimSpace(d.Nama)
	if nama == "" {
		return errors.New("Nama wajib diisi")
	}
	if utf8.RuneCountInString(nama) > 100 {
		return errors.New("Nama maksimal 100 karakter")
	}

	email := NormalisasiEmail(d.Email)
	if !emailValid(email) {
		return errors.New("Format email tidak valid")
	}
	if len(email) > 150 {
		return errors.New("Email maksimal 150 karakter")
	}

	if utf8.RuneCountInString(d.Password) < 8 {
		return errors.New("Password minimal 8 karakter")
	}
	// bcrypt hanya bisa memproses 72 byte pertama, password yang lebih panjang ditolak
	if len(d.Password) > 72 {
		return errors.New("Password terlalu panjang (maksimal 72 karakter)")
	}
	if d.Password != d.Konfirmasi {
		return errors.New("Konfirmasi password tidak sama")
	}
	return nil
}

// emailValid melakukan pemeriksaan sederhana: tanpa spasi, tepat satu "@",
// ada nama sebelum "@", dan bagian domain mengandung titik (mis. kampus.test).
func emailValid(email string) bool {
	if strings.ContainsAny(email, " \t") {
		return false
	}
	bagian := strings.Split(email, "@")
	if len(bagian) != 2 || bagian[0] == "" {
		return false
	}
	domain := bagian[1]
	return strings.Contains(domain, ".") &&
		!strings.HasPrefix(domain, ".") &&
		!strings.HasSuffix(domain, ".")
}

// HashPassword mengubah password menjadi hash bcrypt untuk disimpan di database.
// Password asli tidak pernah disimpan.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// CekPassword mengembalikan true jika password cocok dengan hash di database
func CekPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// BuatToken membuat token sesi acak: 32 byte dari crypto/rand ditulis sebagai 64 karakter hex.
// crypto/rand (bukan math/rand) dipakai karena hasilnya tidak bisa ditebak.
// Sejak Go 1.24, rand.Read tidak pernah mengembalikan error.
func BuatToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// BolehAkses mengembalikan true jika peran pengguna ada di daftar peran yang diizinkan
func BolehAkses(peran string, diizinkan ...string) bool {
	for _, p := range diizinkan {
		if peran == p {
			return true
		}
	}
	return false
}
