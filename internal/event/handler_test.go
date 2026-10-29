package event

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

var (
	admin   = auth.User{ID: 1, Nama: "Admin", Role: auth.PeranAdmin}
	pembeli = auth.User{ID: 2, Nama: "Budi", Role: auth.PeranPembeli}
)

// siapkanMux pindah ke root proyek (template dibaca dari "templates/...") lalu mendaftarkan route event
func siapkanMux(t *testing.T, db *sql.DB) *http.ServeMux {
	t.Chdir("../..")
	mux := http.NewServeMux()
	RegisterRoutes(mux, db)
	return mux
}

// kirim menjalankan satu request sebagai pengguna u (nil berarti tamu)
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

// formEvent berisi isian event yang valid; tanggal 30 hari lagi
func formEvent() url.Values {
	return url.Values{
		"nama":          {"Konser Test"},
		"lokasi":        {"Aula"},
		"kapasitas":     {"100"},
		"tanggal_event": {web.WaktuForm(time.Now().Add(30 * 24 * time.Hour))},
	}
}

func TestHalamanEventHanyaUntukAdmin(t *testing.T) {
	mux := siapkanMux(t, nil) // ditolak sebelum menyentuh database

	if w := kirim(mux, nil, "GET", "/admin/event", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("tamu: dapat %d ke %q, mau 303 ke /login", w.Code, w.Header().Get("Location"))
	}
	if w := kirim(mux, &pembeli, "GET", "/admin/event", nil); w.Code != http.StatusForbidden {
		t.Errorf("pembeli GET: status %d, mau 403", w.Code)
	}
	if w := kirim(mux, &pembeli, "POST", "/admin/event", formEvent()); w.Code != http.StatusForbidden {
		t.Errorf("pembeli POST: status %d, mau 403", w.Code)
	}
}

func TestInputEventTidakValidDibalas400(t *testing.T) {
	mux := siapkanMux(t, nil) // semua kasus ditolak sebelum menyentuh database

	kasus := []struct {
		nama  string
		ubah  func(f url.Values)
		pesan string
	}{
		{"kapasitas bukan angka", func(f url.Values) { f.Set("kapasitas", "abc") }, "Kapasitas harus berupa angka"},
		{"tanggal kosong", func(f url.Values) { f.Set("tanggal_event", "") }, "tidak valid"},
		{"tanggal sudah lewat", func(f url.Values) {
			f.Set("tanggal_event", web.WaktuForm(time.Now().Add(-time.Hour)))
		}, "sudah lewat"},
		{"nama kosong", func(f url.Values) { f.Set("nama", "  ") }, "Nama event wajib"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			f := formEvent()
			k.ubah(f)
			w := kirim(mux, &admin, "POST", "/admin/event", f)
			if w.Code != http.StatusBadRequest {
				t.Errorf("status %d, mau 400", w.Code)
			}
			if !strings.Contains(w.Body.String(), k.pesan) {
				t.Errorf("pesan %q tidak tampil", k.pesan)
			}
			// isian lain tetap terisi
			if k.nama != "nama kosong" && !strings.Contains(w.Body.String(), `value="Konser Test"`) {
				t.Error("isian nama hilang setelah gagal")
			}
		})
	}
}

func TestIdDiAlamatBukanAngkaDibalas400(t *testing.T) {
	mux := siapkanMux(t, nil)
	for _, path := range []string{"/admin/event/abc", "/admin/event/0", "/admin/event/-5/edit"} {
		if w := kirim(mux, &admin, "GET", path, nil); w.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status %d, mau 400", path, w.Code)
		}
	}
}

// Alur admin lewat HTTP: buat event -> kategori -> tahap -> buka penjualan
func TestAlurBuatEventSampaiBukaPenjualan(t *testing.T) {
	db := bukaDBKosong(t)
	mux := siapkanMux(t, db)

	// 1. buat event -> diarahkan ke halaman detail
	w := kirim(mux, &admin, "POST", "/admin/event", formEvent())
	if w.Code != http.StatusSeeOther {
		t.Fatalf("buat event: status %d\n%s", w.Code, w.Body.String())
	}
	var eventID int64
	fmt.Sscanf(w.Header().Get("Location"), "/admin/event/%d", &eventID)
	detail := fmt.Sprintf("/admin/event/%d", eventID)

	// 2. tambah kategori melebihi kapasitas -> 400 (BR-02)
	w = kirim(mux, &admin, "POST", detail+"/kategori", url.Values{"nama": {"Festival"}, "kuota": {"101"}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "melebihi kapasitas") {
		t.Errorf("kategori melebihi kapasitas: status %d", w.Code)
	}

	// 3. tambah kategori 100 -> berhasil
	w = kirim(mux, &admin, "POST", detail+"/kategori", url.Values{"nama": {"Festival"}, "kuota": {"100"}})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("tambah kategori: status %d", w.Code)
	}
	var kategoriID int64
	db.QueryRow("SELECT id FROM ticket_categories WHERE event_id = ?", eventID).Scan(&kategoriID)
	urlTahap := fmt.Sprintf("%s/kategori/%d/tahap", detail, kategoriID)

	tahap := func(nama, kuota string, hariLagi int) url.Values {
		return url.Values{
			"nama": {nama}, "harga": {"150000"}, "kuota": {kuota},
			"batas_tanggal": {web.WaktuForm(time.Now().Add(time.Duration(hariLagi) * 24 * time.Hour))},
		}
	}

	// 4. harga bukan angka -> 400
	salahHarga := tahap("Presale 1", "40", 7)
	salahHarga.Set("harga", "150.000")
	if w = kirim(mux, &admin, "POST", urlTahap, salahHarga); w.Code != http.StatusBadRequest {
		t.Errorf("harga 150.000: status %d, mau 400", w.Code)
	}

	// 5. tahap pertama 40 dari 100 -> buka penjualan ditolak (BR-04), status tetap DRAFT
	if w = kirim(mux, &admin, "POST", urlTahap, tahap("Presale 1", "40", 7)); w.Code != http.StatusSeeOther {
		t.Fatalf("tambah tahap: status %d", w.Code)
	}
	w = kirim(mux, &admin, "POST", detail+"/buka", url.Values{})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "harus sama dengan kuota kategori") {
		t.Errorf("buka penjualan belum lengkap: status %d", w.Code)
	}

	// 6. lengkapi tahap -> buka penjualan berhasil
	if w = kirim(mux, &admin, "POST", urlTahap, tahap("Normal", "60", 20)); w.Code != http.StatusSeeOther {
		t.Fatalf("tambah tahap 2: status %d", w.Code)
	}
	if w = kirim(mux, &admin, "POST", detail+"/buka", url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("buka penjualan: status %d\n%s", w.Code, w.Body.String())
	}
	if s := statusEvent(t, db, eventID); s != StatusOpen {
		t.Errorf("status %s, mau OPEN", s)
	}

	// 7. halaman detail: OPEN, tahap aktif Presale 1, tombol ubah sudah hilang
	w = kirim(mux, &admin, "GET", detail, nil)
	isi := w.Body.String()
	if w.Code != http.StatusOK || !strings.Contains(isi, "<strong>Presale 1</strong>") || strings.Contains(isi, "+ Tambah Kategori") {
		t.Errorf("detail setelah OPEN tidak sesuai (status %d)", w.Code)
	}

	// 8. setelah OPEN, kategori tidak bisa ditambah
	w = kirim(mux, &admin, "POST", detail+"/kategori", url.Values{"nama": {"VIP"}, "kuota": {"1"}})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "sudah dibuka") {
		t.Errorf("tambah kategori saat OPEN: status %d", w.Code)
	}
}

func TestEventTidakAdaDibalas404(t *testing.T) {
	db := bukaDBKosong(t)
	mux := siapkanMux(t, db)

	if w := kirim(mux, &admin, "GET", "/admin/event/999999", nil); w.Code != http.StatusNotFound {
		t.Errorf("status %d, mau 404", w.Code)
	}
	if w := kirim(mux, &admin, "POST", "/admin/event/999999/kategori", url.Values{"nama": {"VIP"}, "kuota": {"1"}}); w.Code != http.StatusNotFound {
		t.Errorf("tambah kategori ke event tidak ada: status %d, mau 404", w.Code)
	}
}
