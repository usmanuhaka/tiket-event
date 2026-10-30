package checkin

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

func siapkanMux(t *testing.T, db *sql.DB) *http.ServeMux {
	t.Chdir("../..")
	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	return mux
}

func kirim(mux *http.ServeMux, u auth.User, metode, path string, isi url.Values) *httptest.ResponseRecorder {
	var r *http.Request
	if isi != nil {
		r = httptest.NewRequest(metode, path, strings.NewReader(isi.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		r = httptest.NewRequest(metode, path, nil)
	}
	r = auth.DenganPengguna(r, u)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// akhirHariIni: hari ini pukul 23:59 WIB, supaya halaman (yang memakai jam sungguhan) menganggapnya hari event
func akhirHariIni() time.Time {
	n := time.Now().In(web.Jakarta)
	return time.Date(n.Year(), n.Month(), n.Day(), 23, 59, 0, 0, web.Jakarta)
}

func TestHalamanPetugasHanyaUntukPetugas(t *testing.T) {
	mux := siapkanMux(t, nil) // ditolak sebelum menyentuh database
	for _, u := range []auth.User{{ID: 1, Role: auth.PeranPembeli}, {ID: 2, Role: auth.PeranAdmin}} {
		if w := kirim(mux, u, "GET", "/petugas", nil); w.Code != http.StatusForbidden {
			t.Errorf("%s membuka /petugas: status %d, mau 403", u.Role, w.Code)
		}
		if w := kirim(mux, u, "POST", "/petugas/event/1/masuk", url.Values{"kode": {"X"}}); w.Code != http.StatusForbidden {
			t.Errorf("%s check-in: status %d, mau 403", u.Role, w.Code)
		}
	}
}

func TestAlurPetugasLewatHTTP(t *testing.T) {
	db := bukaDBKosong(t)
	eventID, tiket := siapkanTiket(t, db, "Konser Hari Ini", akhirHariIni(), 1)
	siapkanTiket(t, db, "Konser Besok", akhirHariIni().AddDate(0, 0, 1), 0)
	petugas := auth.User{ID: buatUser(t, db, "PETUGAS"), Nama: "Petugas", Role: auth.PeranPetugas}
	mux := siapkanMux(t, db)
	halaman := fmt.Sprintf("/petugas/event/%d", eventID)

	// langkah 1: hanya event hari ini
	isi := kirim(mux, petugas, "GET", "/petugas", nil).Body.String()
	if !strings.Contains(isi, "Konser Hari Ini") || strings.Contains(isi, "Konser Besok") {
		t.Error("langkah 1 harus menampilkan hanya event hari ini")
	}

	// langkah 2: kode tidak dikenal
	isi = kirim(mux, petugas, "GET", halaman+"?kode=TKT-TIDAKADA", nil).Body.String()
	if !strings.Contains(isi, "tidak ditemukan") || strings.Contains(isi, "Izinkan Masuk") {
		t.Error("kode tidak dikenal: pesan tidak tampil atau tombol masih ada")
	}

	// langkah 2: kode benar (diketik huruf kecil) -> data tiket dan tombol Izinkan Masuk
	isi = kirim(mux, petugas, "GET", halaman+"?kode="+url.QueryEscape(strings.ToLower(tiket[0].kode)), nil).Body.String()
	for _, mau := range []string{"Pemegang 1", "VIP", "ACTIVE", "Izinkan Masuk"} {
		if !strings.Contains(isi, mau) {
			t.Errorf("hasil periksa tidak memuat %q", mau)
		}
	}

	// Izinkan Masuk -> kembali ke langkah 2 dengan konfirmasi
	w := kirim(mux, petugas, "POST", halaman+"/masuk", url.Values{"kode": {tiket[0].kode}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("masuk: status %d\n%s", w.Code, w.Body.String())
	}
	isi = kirim(mux, petugas, "GET", w.Header().Get("Location"), nil).Body.String()
	if !strings.Contains(isi, "Silakan masuk: Pemegang 1 (VIP)") {
		t.Error("konfirmasi berhasil masuk tidak tampil")
	}

	// dipindai lagi: status USED dan alasan beserta waktunya, tanpa tombol
	isi = kirim(mux, petugas, "GET", halaman+"?kode="+tiket[0].kode, nil).Body.String()
	if !strings.Contains(isi, "USED") || !strings.Contains(isi, "sudah dipakai masuk pada") || strings.Contains(isi, "Izinkan Masuk") {
		t.Error("pindai kedua: status/alasan tidak tampil atau tombol masih ada")
	}
	// tombol lama dikirim ulang (mis. dua petugas) -> 400 dengan alasan
	w = kirim(mux, petugas, "POST", halaman+"/masuk", url.Values{"kode": {tiket[0].kode}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "sudah dipakai masuk pada") {
		t.Errorf("masuk kedua: status %d", w.Code)
	}

	if w := kirim(mux, petugas, "GET", "/petugas/event/999999", nil); w.Code != http.StatusNotFound {
		t.Errorf("event tidak ada: status %d, mau 404", w.Code)
	}
}
