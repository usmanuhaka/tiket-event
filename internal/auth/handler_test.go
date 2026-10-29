package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// keRootProyek pindah ke root proyek selama test, karena template dibaca dari "templates/..."
func keRootProyek(t *testing.T) {
	t.Chdir("../..")
}

// denganPengguna meniru hasil MuatPengguna: request membawa pengguna yang sudah login
func denganPengguna(r *http.Request, u User) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), kunciPengguna, u))
}

func halamanOK(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("isi halaman"))
}

func TestWajib(t *testing.T) {
	keRootProyek(t)
	halamanAdmin := Wajib(halamanOK, PeranAdmin)
	halamanLogin := Wajib(halamanOK) // tanpa peran: semua yang login boleh

	t.Run("tamu diarahkan ke login", func(t *testing.T) {
		w := httptest.NewRecorder()
		halamanAdmin(w, httptest.NewRequest("GET", "/admin", nil))

		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
			t.Errorf("dapat %d ke %q, mau 303 ke /login", w.Code, w.Header().Get("Location"))
		}
	})

	t.Run("pembeli di halaman admin ditolak 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := denganPengguna(httptest.NewRequest("GET", "/admin", nil), User{Nama: "Budi", Role: PeranPembeli})
		halamanAdmin(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("status %d, mau 403", w.Code)
		}
		if !strings.Contains(w.Body.String(), "tidak punya akses") {
			t.Error("halaman 403 tidak menampilkan pesan")
		}
	})

	t.Run("admin di halaman admin boleh", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := denganPengguna(httptest.NewRequest("GET", "/admin", nil), User{Nama: "Admin", Role: PeranAdmin})
		halamanAdmin(w, r)

		if w.Code != http.StatusOK || w.Body.String() != "isi halaman" {
			t.Errorf("status %d, isi %q", w.Code, w.Body.String())
		}
	})

	t.Run("tanpa daftar peran, pengguna login mana pun boleh", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := denganPengguna(httptest.NewRequest("GET", "/akun", nil), User{Nama: "Petugas", Role: PeranPetugas})
		halamanLogin(w, r)

		if w.Code != http.StatusOK {
			t.Errorf("status %d, mau 200", w.Code)
		}
	})
}

func TestHalamanLoginUntukYangSudahLogin(t *testing.T) {
	keRootProyek(t)
	mux := http.NewServeMux()
	RegisterRoutes(mux, nil) // GET /login dan GET /daftar tidak memakai database

	for _, path := range []string{"/login", "/daftar"} {
		w := httptest.NewRecorder()
		r := denganPengguna(httptest.NewRequest("GET", path, nil), User{Nama: "Budi", Role: PeranPembeli})
		mux.ServeHTTP(w, r)

		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
			t.Errorf("GET %s: dapat %d ke %q, mau 303 ke /", path, w.Code, w.Header().Get("Location"))
		}
	}
}
