package report

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usmanuhaka/tiket-event/internal/auth"
)

func siapkanMux(t *testing.T, db *sql.DB) *http.ServeMux {
	t.Chdir("../..")
	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	return mux
}

func buka(mux *http.ServeMux, u auth.User, path string) *httptest.ResponseRecorder {
	r := auth.DenganPengguna(httptest.NewRequest("GET", path, nil), u)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

var admin = auth.User{ID: 1, Nama: "Admin", Role: auth.PeranAdmin}

func TestLaporanHanyaUntukAdmin(t *testing.T) {
	mux := siapkanMux(t, nil) // ditolak sebelum menyentuh database
	for _, peran := range []string{auth.PeranPembeli, auth.PeranPetugas} {
		if w := buka(mux, auth.User{ID: 2, Role: peran}, "/admin/laporan"); w.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, mau 403", peran, w.Code)
		}
		if w := buka(mux, auth.User{ID: 2, Role: peran}, "/admin/laporan/1"); w.Code != http.StatusForbidden {
			t.Errorf("%s detail: status %d, mau 403", peran, w.Code)
		}
	}
	if w := buka(mux, admin, "/admin/laporan/abc"); w.Code != http.StatusBadRequest {
		t.Errorf("id bukan angka: status %d, mau 400", w.Code)
	}
}

func TestHalamanLaporanLewatHTTP(t *testing.T) {
	db := bukaDBKosong(t)
	u := buatPembeli(t, db)
	eventID := buatEvent(t, db, "Konser Laporan")
	k := buatKategori(t, db, eventID, "Festival")
	tahap := buatTahap(t, db, k, "Presale", 1, 150000, 10)
	buatTiket(t, db, u, eventID, k, tahap, 150000, 22500, "PAID", "ACTIVE")
	buatTiket(t, db, u, eventID, k, tahap, 150000, 0, "PAID", "USED")
	mux := siapkanMux(t, db)

	isi := buka(mux, admin, "/admin/laporan").Body.String()
	if !strings.Contains(isi, "Konser Laporan") || !strings.Contains(isi, "Rp277.500") {
		t.Error("daftar laporan tidak menampilkan event dan bersihnya (300.000 - 22.500)")
	}

	w := buka(mux, admin, fmt.Sprintf("/admin/laporan/%d", eventID))
	if w.Code != http.StatusOK {
		t.Fatalf("detail: status %d", w.Code)
	}
	isi = w.Body.String()
	for _, mau := range []string{"Festival / Presale", "Subtotal Festival", "TOTAL EVENT", "Rp300.000", "Rp22.500", "Rp277.500"} {
		if !strings.Contains(isi, mau) {
			t.Errorf("laporan tidak memuat %q", mau)
		}
	}

	if w := buka(mux, admin, "/admin/laporan/999999"); w.Code != http.StatusNotFound {
		t.Errorf("event tidak ada: status %d, mau 404", w.Code)
	}
}
