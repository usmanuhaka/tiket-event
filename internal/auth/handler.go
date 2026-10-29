package auth

import (
	"context"
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/web"
)

// namaCookie adalah nama cookie di browser yang menyimpan token sesi
const namaCookie = "sesi"

// kunciKonteks dibuat sebagai tipe sendiri supaya kunci di context
// tidak bisa bertabrakan dengan kunci dari paket lain
type kunciKonteks string

const kunciPengguna kunciKonteks = "pengguna"

// dataLogin adalah data untuk templates/auth/login.html
type dataLogin struct {
	web.Halaman
	Email string
}

// dataDaftar adalah data untuk templates/auth/daftar.html.
// NamaBaru (bukan Nama) karena Nama sudah dipakai web.Halaman untuk pengguna yang login.
type dataDaftar struct {
	web.Halaman
	NamaBaru string
	Email    string
}

// RegisterRoutes mendaftarkan route login, daftar, dan logout
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tplLogin := web.ParseHalaman("auth/login.html")
	tplDaftar := web.ParseHalaman("auth/daftar.html")

	mux.HandleFunc("GET /login", loginFormHandler(tplLogin))
	mux.HandleFunc("POST /login", loginHandler(db, tplLogin))
	mux.HandleFunc("GET /daftar", daftarFormHandler(tplDaftar))
	mux.HandleFunc("POST /daftar", daftarHandler(db, tplDaftar))
	mux.HandleFunc("POST /logout", logoutHandler(db))
}

// ===== Middleware =====

// MuatPengguna membungkus semua route. Di setiap request ia membaca cookie sesi,
// mencari pemiliknya di tabel sessions, lalu menitipkan data pengguna di context request.
// Jika tidak ada cookie atau sesinya tidak berlaku, request diteruskan sebagai tamu.
func MuatPengguna(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(namaCookie)
		if err == nil {
			u, err := getUserBySessionToken(r.Context(), db, cookie.Value, time.Now())
			if err == nil {
				r = r.WithContext(context.WithValue(r.Context(), kunciPengguna, u))
			} else if !errors.Is(err, sql.ErrNoRows) {
				log.Printf("cek sesi: %v", err)
				http.Error(w, "Gagal memeriksa sesi login", http.StatusInternalServerError)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Wajib membatasi halaman untuk pengguna yang sudah login dengan peran tertentu.
// Tamu diarahkan ke /login, peran yang tidak diizinkan mendapat halaman 403.
// Jika daftar peran kosong, semua pengguna yang sudah login boleh masuk.
func Wajib(next http.HandlerFunc, peran ...string) http.HandlerFunc {
	tplPesan := web.ParseHalaman("pesan.html")

	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := PenggunaDari(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if len(peran) > 0 && !BolehAkses(u.Role, peran...) {
			web.RenderStatus(w, tplPesan, web.DataPesan{
				Halaman: Halaman(r),
				Judul:   "Akses ditolak",
				Isi:     "Anda tidak punya akses ke halaman ini.",
			}, http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// PenggunaDari mengambil pengguna yang dititipkan MuatPengguna.
// ok bernilai false jika pengunjung belum login.
func PenggunaDari(r *http.Request) (User, bool) {
	u, ok := r.Context().Value(kunciPengguna).(User)
	return u, ok
}

// Halaman mengisi data layout (nama dan peran di header/sidebar) dari pengguna yang login
func Halaman(r *http.Request) web.Halaman {
	u, ok := PenggunaDari(r)
	if !ok {
		return web.Halaman{}
	}
	return web.Halaman{Nama: u.Nama, Peran: u.Role}
}

// ===== Handler =====

func loginFormHandler(tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// yang sudah login tidak perlu melihat form login
		if _, ok := PenggunaDari(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		data := dataLogin{Halaman: Halaman(r)}
		if r.URL.Query().Get("daftar") == "ok" {
			data.Pesan = "Pendaftaran berhasil, silakan login."
		}
		web.Render(w, tpl, data)
	}
}

func loginHandler(db *sql.DB, tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := NormalisasiEmail(r.PostFormValue("email"))
		password := r.PostFormValue("password")

		u, err := getUserByEmail(r.Context(), db, email)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			log.Printf("login: %v", err)
			http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
			return
		}

		// pesan yang sama untuk email tidak terdaftar dan password salah,
		// supaya orang lain tidak bisa menebak email mana yang terdaftar
		if errors.Is(err, sql.ErrNoRows) || !CekPassword(u.PasswordHash, password) {
			data := dataLogin{Halaman: Halaman(r), Email: email}
			data.Error = "Email atau password salah"
			web.RenderStatus(w, tpl, data, http.StatusUnauthorized)
			return
		}

		// buat sesi baru: token acak disimpan di tabel sessions dan di cookie browser
		token := BuatToken()
		berlakuSampai := time.Now().Add(LamaSesi)
		if err := simpanSession(r.Context(), db, token, u.ID, berlakuSampai); err != nil {
			log.Printf("simpan sesi: %v", err)
			http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     namaCookie,
			Value:    token,
			Path:     "/",
			Expires:  berlakuSampai,
			HttpOnly: true,                 // tidak bisa dibaca JavaScript
			SameSite: http.SameSiteLaxMode, // tidak dikirim pada POST dari situs lain
		})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func daftarFormHandler(tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := PenggunaDari(r); ok {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		web.Render(w, tpl, dataDaftar{Halaman: Halaman(r)})
	}
}

func daftarHandler(db *sql.DB, tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d := DataDaftar{
			Nama:       r.PostFormValue("nama"),
			Email:      r.PostFormValue("email"),
			Password:   r.PostFormValue("password"),
			Konfirmasi: r.PostFormValue("konfirmasi"),
		}

		// tampilkan form lagi dengan pesan error; nama dan email tetap terisi
		tampilGagal := func(pesan string) {
			data := dataDaftar{Halaman: Halaman(r), NamaBaru: d.Nama, Email: d.Email}
			data.Error = pesan
			web.RenderStatus(w, tpl, data, http.StatusBadRequest)
		}

		if err := ValidasiDaftar(d); err != nil {
			tampilGagal(err.Error())
			return
		}

		hash, err := HashPassword(d.Password)
		if err != nil {
			log.Printf("hash password: %v", err)
			http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
			return
		}

		// pendaftaran lewat halaman ini selalu menjadi PEMBELI;
		// akun admin dan petugas dibuat lewat data contoh di migrations/
		u := User{
			Nama:         strings.TrimSpace(d.Nama),
			Email:        NormalisasiEmail(d.Email),
			PasswordHash: hash,
			Role:         PeranPembeli,
		}
		_, err = simpanUser(r.Context(), db, u)
		if errors.Is(err, ErrEmailTerdaftar) {
			tampilGagal(err.Error())
			return
		}
		if err != nil {
			log.Printf("simpan user: %v", err)
			http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/login?daftar=ok", http.StatusSeeOther)
	}
}

func logoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// hapus sesi di database supaya token lama tidak berlaku lagi
		if cookie, err := r.Cookie(namaCookie); err == nil {
			if err := hapusSession(r.Context(), db, cookie.Value); err != nil {
				log.Printf("hapus sesi: %v", err)
				http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
				return
			}
		}

		// MaxAge -1 menyuruh browser menghapus cookie
		http.SetCookie(w, &http.Cookie{
			Name:     namaCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
