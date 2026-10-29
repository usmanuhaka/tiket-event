package order

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

// siapkanMux pindah ke root proyek (template dibaca dari "templates/...") lalu mendaftarkan route pesanan
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

func TestPesanHanyaUntukPembeli(t *testing.T) {
	mux := siapkanMux(t, nil) // ditolak sebelum menyentuh database
	admin := auth.User{ID: 1, Nama: "Admin", Role: auth.PeranAdmin}
	form := url.Values{"kategori_1": {"1"}, "nama_1": {"Budi"}}

	if w := kirim(mux, nil, "POST", "/event/1/pesan", form); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("tamu pesan: %d ke %q, mau 303 ke /login", w.Code, w.Header().Get("Location"))
	}
	if w := kirim(mux, &admin, "POST", "/event/1/pesan", form); w.Code != http.StatusForbidden {
		t.Errorf("admin pesan: status %d, mau 403", w.Code)
	}
	if w := kirim(mux, nil, "GET", "/pesanan", nil); w.Code != http.StatusSeeOther {
		t.Errorf("tamu buka pesanan: status %d, mau 303", w.Code)
	}
}

func TestKatalogHanyaEventYangDibuka(t *testing.T) {
	db := bukaDBKosong(t)
	dibuka := siapkanEvent(t, db, []tahapUji{{"Presale", 100000, 5, 5}})
	draft := siapkanEvent(t, db, []tahapUji{{"Normal", 100000, 5, 5}})
	lewat := siapkanEvent(t, db, []tahapUji{{"Normal", 100000, 5, 5}})
	db.Exec("UPDATE events SET nama = 'Event Draft', status = 'DRAFT' WHERE id = ?", draft.eventID)
	db.Exec("UPDATE events SET nama = 'Event Lewat', tanggal_event = ? WHERE id = ?", time.Now().Add(-time.Hour), lewat.eventID)
	mux := siapkanMux(t, db)

	isi := kirim(mux, nil, "GET", "/event", nil).Body.String()
	if !strings.Contains(isi, fmt.Sprintf("/event/%d", dibuka.eventID)) || strings.Contains(isi, "Event Draft") || strings.Contains(isi, "Event Lewat") {
		t.Error("katalog harus menampilkan hanya event OPEN yang belum lewat")
	}

	// tamu bisa melihat harga tahap aktif, tetapi diminta login untuk memesan
	w := kirim(mux, nil, "GET", fmt.Sprintf("/event/%d", dibuka.eventID), nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Rp100.000") || !strings.Contains(w.Body.String(), "untuk memesan tiket") {
		t.Errorf("halaman event untuk tamu tidak sesuai (status %d)", w.Code)
	}
	if w := kirim(mux, nil, "GET", fmt.Sprintf("/event/%d", draft.eventID), nil); w.Code != http.StatusNotFound {
		t.Errorf("event DRAFT: status %d, mau 404", w.Code)
	}
}

func TestFormPesanTidakValidDibalas400(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 2, 20}})
	budi := auth.User{ID: buatPembeli(t, db), Nama: "Budi", Role: auth.PeranPembeli}
	mux := siapkanMux(t, db)
	path := fmt.Sprintf("/event/%d/pesan", e.eventID)
	kategori := fmt.Sprint(e.kategoriID)

	kasus := []struct {
		nama  string
		form  url.Values
		pesan string
	}{
		{"BR-06 tanpa tiket", url.Values{}, "Isi minimal 1 tiket"},
		{"BR-07 nama kosong", url.Values{"kategori_1": {kategori}, "nama_1": {" "}}, "Nama pemegang tiket ke-1"},
		{"nama tanpa kategori", url.Values{"nama_2": {"Ani"}}, "Tiket ke-2: pilih kategori"},
		{"kategori bukan angka", url.Values{"kategori_1": {"abc"}, "nama_1": {"Ani"}}, "kategori tidak valid"},
		{"kuota kurang", url.Values{
			"kategori_1": {kategori}, "nama_1": {"Ani"},
			"kategori_2": {kategori}, "nama_2": {"Budi"},
			"kategori_3": {kategori}, "nama_3": {"Cici"},
		}, "Kuota tidak cukup"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			w := kirim(mux, &budi, "POST", path, k.form)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), k.pesan) {
				t.Errorf("status %d, pesan %q tidak tampil", w.Code, k.pesan)
			}
		})
	}

	// isian tetap terisi setelah gagal
	w := kirim(mux, &budi, "POST", path, url.Values{"kategori_1": {kategori}, "nama_1": {"Ani"}, "nama_2": {"Tanpa Kategori"}})
	if !strings.Contains(w.Body.String(), `value="Tanpa Kategori"`) || !strings.Contains(w.Body.String(), "selected") {
		t.Error("isian form hilang setelah gagal")
	}
}

// Alur pembeli lewat HTTP: pesan -> lihat pesanan -> bayar -> kode tiket tampil
func TestAlurPesanSampaiBayar(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Presale", 100000, 1, 5}, {"Normal", 150000, 5, 20}})
	budi := auth.User{ID: buatPembeli(t, db), Nama: "Budi", Role: auth.PeranPembeli}
	ani := auth.User{ID: buatPembeli(t, db), Nama: "Ani", Role: auth.PeranPembeli}
	mux := siapkanMux(t, db)
	kategori := fmt.Sprint(e.kategoriID)

	// 1. pesan 2 tiket -> diarahkan ke detail pesanan
	w := kirim(mux, &budi, "POST", fmt.Sprintf("/event/%d/pesan", e.eventID), url.Values{
		"kategori_1": {kategori}, "nama_1": {"Budi"},
		"kategori_2": {kategori}, "nama_2": {"Teman Budi"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("pesan: status %d\n%s", w.Code, w.Body.String())
	}
	detail := w.Header().Get("Location")

	// 2. detail: HELD, tombol bayar ada, harga BR-08 (Presale lalu Normal)
	isi := kirim(mux, &budi, "GET", detail, nil).Body.String()
	for _, mau := range []string{"HELD", "Konfirmasi Pembayaran", "Rp100.000", "Rp150.000", "Rp250.000"} {
		if !strings.Contains(isi, mau) {
			t.Errorf("detail pesanan tidak memuat %q", mau)
		}
	}

	// 3. pembeli lain tidak bisa melihat atau membayar
	if w := kirim(mux, &ani, "GET", detail, nil); w.Code != http.StatusNotFound {
		t.Errorf("pembeli lain melihat pesanan: status %d, mau 404", w.Code)
	}
	if w := kirim(mux, &ani, "POST", detail+"/bayar", url.Values{}); w.Code != http.StatusNotFound {
		t.Errorf("pembeli lain membayar: status %d, mau 404", w.Code)
	}

	// 4. bayar -> PAID dan kode tiket tampil
	if w := kirim(mux, &budi, "POST", detail+"/bayar", url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("bayar: status %d", w.Code)
	}
	isi = kirim(mux, &budi, "GET", detail, nil).Body.String()
	if !strings.Contains(isi, "PAID") || strings.Count(isi, `class="kode">TKT-`) != 2 || strings.Contains(isi, "Konfirmasi Pembayaran") {
		t.Error("setelah bayar: harus PAID, 2 kode tiket, tanpa tombol bayar")
	}

	// 5. bayar lagi -> ditolak dengan pesan
	if w := kirim(mux, &budi, "POST", detail+"/bayar", url.Values{}); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "sudah dibayar") {
		t.Errorf("bayar dua kali: status %d", w.Code)
	}

	// 6. daftar pesanan saya
	if isi := kirim(mux, &budi, "GET", "/pesanan", nil).Body.String(); !strings.Contains(isi, detail) {
		t.Error("pesanan tidak muncul di daftar Pesanan Saya")
	}
}

func TestBayarTerlambatLewatHTTP(t *testing.T) {
	db := bukaDBKosong(t)
	e := siapkanEvent(t, db, []tahapUji{{"Normal", 150000, 5, 20}})
	budi := auth.User{ID: buatPembeli(t, db), Nama: "Budi", Role: auth.PeranPembeli}
	mux := siapkanMux(t, db)

	// pesanan dibuat 16 menit lalu
	orderID := pesanSatu(t, e, budi.ID, 1, waktuSekarang().Add(-16*time.Minute), db)
	detail := fmt.Sprintf("/pesanan/%d", orderID)

	// sebelum worker jalan: tombol bayar sudah disembunyikan
	if isi := kirim(mux, &budi, "GET", detail, nil).Body.String(); strings.Contains(isi, "Konfirmasi Pembayaran") {
		t.Error("tombol bayar masih tampil padahal sudah lewat 15 menit")
	}
	// jika tetap dikirim (mis. halaman lama), pembayaran ditolak dan pesanan EXPIRED
	w := kirim(mux, &budi, "POST", detail+"/bayar", url.Values{})
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "sudah habis") {
		t.Errorf("bayar terlambat: status %d", w.Code)
	}
	if s := statusPesanan(t, db, orderID); s != StatusExpired {
		t.Errorf("status %s, mau EXPIRED", s)
	}
}
