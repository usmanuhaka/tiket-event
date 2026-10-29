package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// kirimForm mengirim POST berisi form, seperti browser menekan tombol submit
func kirimForm(h http.Handler, path string, isi url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(isi.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func buka(h http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// cookieSesi mengambil cookie "sesi" dari jawaban server, atau nil jika tidak ada
func cookieSesi(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == namaCookie {
			return c
		}
	}
	return nil
}

// Alur lengkap: daftar -> login -> buka halaman terbatas -> logout -> halaman terbatas ditolak
func TestAlurDaftarLoginLogout(t *testing.T) {
	db := bukaDBKosong(t)
	keRootProyek(t)

	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	mux.HandleFunc("GET /khusus-pembeli", Wajib(halamanOK, PeranPembeli))
	app := MuatPengguna(db, mux)

	hitungSesi := func() int {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	formDaftar := url.Values{
		"nama": {"Budi"}, "email": {"Budi@Kampus.test"},
		"password": {"rahasia123"}, "konfirmasi": {"rahasia123"},
	}

	// 1. daftar berhasil -> diarahkan ke halaman login
	w := kirimForm(app, "/daftar", formDaftar, nil)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login?daftar=ok" {
		t.Fatalf("daftar: dapat %d ke %q", w.Code, w.Header().Get("Location"))
	}

	// 2. daftar lagi dengan email yang sama -> ditolak
	w = kirimForm(app, "/daftar", formDaftar, nil)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Email sudah terdaftar") {
		t.Errorf("daftar ganda: status %d", w.Code)
	}

	// 3. login dengan password salah -> 401, tanpa cookie
	w = kirimForm(app, "/login", url.Values{"email": {"budi@kampus.test"}, "password": {"salah12345"}}, nil)
	if w.Code != http.StatusUnauthorized || cookieSesi(w) != nil {
		t.Errorf("login salah: status %d, cookie %v", w.Code, cookieSesi(w))
	}
	if !strings.Contains(w.Body.String(), "Email atau password salah") {
		t.Error("login salah: pesan error tidak tampil")
	}

	// 4. login benar -> cookie sesi terpasang dan satu baris masuk ke tabel sessions
	w = kirimForm(app, "/login", url.Values{"email": {"budi@kampus.test"}, "password": {"rahasia123"}}, nil)
	sesi := cookieSesi(w)
	if w.Code != http.StatusSeeOther || sesi == nil {
		t.Fatalf("login: status %d, cookie %v", w.Code, sesi)
	}
	if len(sesi.Value) != 64 || !sesi.HttpOnly {
		t.Errorf("cookie tidak sesuai: %+v", sesi)
	}
	if n := hitungSesi(); n != 1 {
		t.Errorf("jumlah sesi %d, mau 1", n)
	}

	// 5. dengan cookie itu, halaman khusus pembeli bisa dibuka
	w = buka(app, "/khusus-pembeli", sesi)
	if w.Code != http.StatusOK {
		t.Errorf("halaman pembeli: status %d, mau 200", w.Code)
	}

	// 6. logout -> sesi dihapus dari database dan cookie dihapus di browser
	w = kirimForm(app, "/logout", url.Values{}, sesi)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("logout: dapat %d ke %q", w.Code, w.Header().Get("Location"))
	}
	if c := cookieSesi(w); c == nil || c.MaxAge >= 0 {
		t.Errorf("logout tidak menghapus cookie: %+v", c)
	}
	if n := hitungSesi(); n != 0 {
		t.Errorf("jumlah sesi setelah logout %d, mau 0", n)
	}

	// 7. cookie lama sudah tidak berlaku
	w = buka(app, "/khusus-pembeli", sesi)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("cookie lama masih berlaku: status %d", w.Code)
	}
}
