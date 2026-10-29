package ticket

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
)

var admin = auth.User{ID: 1, Nama: "Admin", Role: auth.PeranAdmin}

func siapkanMux(t *testing.T, db *sql.DB) *http.ServeMux {
	t.Chdir("../..")
	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	return mux
}

func kirim(mux *http.ServeMux, u *auth.User, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(url.Values{}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if u != nil {
		r = auth.DenganPengguna(r, *u)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

func TestPembatalanDibatasiPerPeran(t *testing.T) {
	mux := siapkanMux(t, nil) // ditolak sebelum menyentuh database
	pembeli := auth.User{ID: 2, Nama: "Budi", Role: auth.PeranPembeli}

	if w := kirim(mux, &pembeli, "/admin/event/1/batal"); w.Code != http.StatusForbidden {
		t.Errorf("pembeli membatalkan event: status %d, mau 403", w.Code)
	}
	if w := kirim(mux, &admin, "/tiket/1/batal"); w.Code != http.StatusForbidden {
		t.Errorf("admin membatalkan tiket: status %d, mau 403", w.Code)
	}
	if w := kirim(mux, nil, "/tiket/1/batal"); w.Code != http.StatusSeeOther {
		t.Errorf("tamu: status %d, mau 303 ke login", w.Code)
	}
	if w := kirim(mux, &pembeli, "/tiket/abc/batal"); w.Code != http.StatusBadRequest {
		t.Errorf("id bukan angka: status %d, mau 400", w.Code)
	}
}

func TestAlurBatalTiketLewatHTTP(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	budi := auth.User{ID: buatPembeli(t, db), Nama: "Budi", Role: auth.PeranPembeli}
	ani := auth.User{ID: buatPembeli(t, db), Nama: "Ani", Role: auth.PeranPembeli}
	p := buatPesanan(t, db, budi.ID, e, "PAID", 1, 0, 0, tanpaPromo())
	mux := siapkanMux(t, db)
	path := fmt.Sprintf("/tiket/%d/batal", p.tiketID[0])
	halamanPesanan := fmt.Sprintf("/pesanan/%d", p.orderID)

	// pembeli lain -> 404
	if w := kirim(mux, &ani, path); w.Code != http.StatusNotFound {
		t.Errorf("pembeli lain: status %d, mau 404", w.Code)
	}
	// pemilik -> kembali ke halaman pesanan
	w := kirim(mux, &budi, path)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != halamanPesanan {
		t.Fatalf("batal: %d ke %q", w.Code, w.Header().Get("Location"))
	}
	// batal lagi -> 400 dengan alasan dan tombol kembali ke pesanan
	w = kirim(mux, &budi, path)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "REFUNDED") || !strings.Contains(w.Body.String(), `href="`+halamanPesanan+`"`) {
		t.Errorf("batal kedua: status %d", w.Code)
	}
}

func TestAlurBatalEventLewatHTTP(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, time.Now().Add(hari(30)), []tahapUji{{"Normal", 150000, 10, 20}})
	budi := buatPembeli(t, db)
	p := buatPesanan(t, db, budi, e, "PAID", 2, 0, 0, tanpaPromo())
	mux := siapkanMux(t, db)
	path := fmt.Sprintf("/admin/event/%d/batal", e.eventID)

	w := kirim(mux, &admin, path)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != fmt.Sprintf("/admin/event/%d", e.eventID) {
		t.Fatalf("batal event: %d ke %q", w.Code, w.Header().Get("Location"))
	}
	if s := teks(t, db, "SELECT status FROM events WHERE id = ?", e.eventID); s != "CANCELLED" {
		t.Errorf("status event %s", s)
	}
	if n := hitung(t, db, "SELECT COUNT(*) FROM refunds WHERE order_id = ? AND persen = 100", p.orderID); n != 2 {
		t.Errorf("refund 100%% = %d, mau 2", n)
	}
	// batal lagi -> 400
	if w := kirim(mux, &admin, path); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "CANCELLED") {
		t.Errorf("batal event kedua: status %d", w.Code)
	}
	if w := kirim(mux, &admin, "/admin/event/999999/batal"); w.Code != http.StatusNotFound {
		t.Errorf("event tidak ada: status %d, mau 404", w.Code)
	}
}
