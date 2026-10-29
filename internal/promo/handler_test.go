package promo

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/usmanuhaka/tiket-event/internal/auth"
)

var (
	admin   = auth.User{ID: 1, Nama: "Admin", Role: auth.PeranAdmin}
	pembeli = auth.User{ID: 2, Nama: "Budi", Role: auth.PeranPembeli}
)

func siapkanMux(t *testing.T, db *sql.DB) *http.ServeMux {
	t.Chdir("../..")
	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	return mux
}

func kirim(mux *http.ServeMux, u *auth.User, metode, path string, isi url.Values) *httptest.ResponseRecorder {
	var r *http.Request
	if isi != nil {
		r = httptest.NewRequest(metode, path, strings.NewReader(isi.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(metode, path, nil)
	}
	if u != nil {
		r = auth.DenganPengguna(r, *u)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestHalamanPromoHanyaUntukAdmin(t *testing.T) {
	mux := siapkanMux(t, nil) // ditolak sebelum menyentuh database
	if w := kirim(mux, &pembeli, "GET", "/admin/promo", nil); w.Code != http.StatusForbidden {
		t.Errorf("pembeli: status %d, mau 403", w.Code)
	}
	if w := kirim(mux, nil, "POST", "/admin/promo", url.Values{}); w.Code != http.StatusSeeOther {
		t.Errorf("tamu: status %d, mau 303 ke login", w.Code)
	}
}

func TestAlurAdminTambahDanHapusPromo(t *testing.T) {
	db := bukaDBKosong(t)
	vip := buatKategori(t, db, "Konser", "OPEN", "VIP")
	mux := siapkanMux(t, db)
	form := func(kode, persen, kuota string, kategori ...int64) url.Values {
		f := url.Values{"kode": {kode}, "persen": {persen}, "kuota": {kuota}}
		for _, k := range kategori {
			f.Add("kategori", fmt.Sprint(k))
		}
		return f
	}

	// isian salah -> 400, isian lain tetap terisi
	kasus := []struct {
		nama  string
		form  url.Values
		pesan string
	}{
		{"persen 0", form("HEMAT", "0", "10", vip), "antara 1 dan 100"},
		{"persen bukan angka", form("HEMAT", "abc", "10", vip), "harus berupa angka"},
		{"tanpa kategori", form("HEMAT", "15", "10"), "minimal 1 kategori"},
		{"kode berspasi", form("HE MAT", "15", "10", vip), "hanya boleh berisi"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			w := kirim(mux, &admin, "POST", "/admin/promo", k.form)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), k.pesan) {
				t.Errorf("status %d, pesan %q tidak tampil", w.Code, k.pesan)
			}
		})
	}

	// berhasil -> tampil di daftar dengan kode huruf besar
	if w := kirim(mux, &admin, "POST", "/admin/promo", form("hemat15", "15", "10", vip)); w.Code != http.StatusSeeOther {
		t.Fatalf("simpan: status %d\n%s", w.Code, w.Body.String())
	}
	isi := kirim(mux, &admin, "GET", "/admin/promo", nil).Body.String()
	if !strings.Contains(isi, "HEMAT15") || !strings.Contains(isi, "0 / 10") || !strings.Contains(isi, "Konser - VIP") {
		t.Error("promo baru tidak tampil lengkap di daftar")
	}

	// kode ganda -> 400, kotak centang tetap tercentang
	w := kirim(mux, &admin, "POST", "/admin/promo", form("HEMAT15", "20", "5", vip))
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "sudah dipakai") || !strings.Contains(w.Body.String(), "checked") {
		t.Errorf("kode ganda: status %d", w.Code)
	}

	// hapus -> hilang dari daftar; hapus lagi -> 404
	var id int64
	db.QueryRow("SELECT id FROM promo_codes WHERE kode = 'HEMAT15'").Scan(&id)
	hapus := fmt.Sprintf("/admin/promo/%d/hapus", id)
	if w := kirim(mux, &admin, "POST", hapus, url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("hapus: status %d", w.Code)
	}
	if strings.Contains(kirim(mux, &admin, "GET", "/admin/promo", nil).Body.String(), "HEMAT15") {
		t.Error("promo yang dihapus masih tampil")
	}
	if w := kirim(mux, &admin, "POST", hapus, url.Values{}); w.Code != http.StatusNotFound {
		t.Errorf("hapus dua kali: status %d, mau 404", w.Code)
	}
}
