package checkin

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/event"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// halaman mengumpulkan template fitur pemeriksaan tiket
type halaman struct {
	pilih   *template.Template
	periksa *template.Template
	pesan   *template.Template
}

// dataPilih adalah data untuk templates/checkin/pilih.html (langkah 1)
type dataPilih struct {
	web.Halaman
	Event []event.Event
}

// dataPeriksa adalah data untuk templates/checkin/periksa.html (langkah 2)
type dataPeriksa struct {
	web.Halaman
	Event      event.Event
	Kode       string    // isian kolom kode
	Ada        bool      // tiket ditemukan, tampilkan datanya (BR-24)
	Tiket      InfoTiket // nama pemegang, kategori, status
	BolehMasuk bool      // tampilkan tombol "Izinkan Masuk"
	Alasan     string    // alasan penolakan
}

// RegisterRoutes mendaftarkan halaman pemeriksaan tiket. Semuanya khusus PETUGAS.
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tpl := halaman{
		pilih:   web.ParseHalaman("checkin/pilih.html"),
		periksa: web.ParseHalaman("checkin/periksa.html"),
		pesan:   web.ParseHalaman("pesan.html"),
	}

	petugas := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.Wajib(h, auth.PeranPetugas)
	}

	mux.HandleFunc("GET /petugas", petugas(pilihEventHandler(db, tpl)))
	mux.HandleFunc("GET /petugas/event/{id}", petugas(periksaHandler(db, tpl)))
	mux.HandleFunc("POST /petugas/event/{id}/masuk", petugas(masukHandler(db, tpl)))
}

// pilihEventHandler (langkah 1): daftar event OPEN yang berlangsung hari ini
func pilihEventHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		daftar, err := ambilEventHariIni(r.Context(), db, time.Now())
		if err != nil {
			gagalServer(w, "ambil event hari ini", err)
			return
		}
		web.Render(w, tpl.pilih, dataPilih{Halaman: auth.Halaman(r), Event: daftar})
	}
}

// periksaHandler (langkah 2): menampilkan data tiket dari kode yang diketik, tanpa mengubah apa pun.
// Setelah check-in berhasil, halaman ini dibuka lagi dengan ?berhasil=KODE untuk menampilkan konfirmasi.
func periksaHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, ok := eventDariPath(w, r, db, tpl)
		if !ok {
			return
		}
		data := dataPeriksa{Halaman: auth.Halaman(r), Event: e}

		if berhasil := r.URL.Query().Get("berhasil"); berhasil != "" {
			if t, err := periksaKode(r.Context(), db, berhasil); err == nil {
				data.Pesan = fmt.Sprintf("Silakan masuk: %s (%s), tercatat %s.", t.NamaPemegang, t.NamaKategori, web.Tanggal(t.WaktuMasuk))
			}
		}

		if kode := r.URL.Query().Get("kode"); kode != "" {
			data.Kode = kode
			isiHasilPeriksa(r, db, &data, kode, e.ID)
		}
		web.Render(w, tpl.periksa, data)
	}
}

// isiHasilPeriksa mencari kode dan menentukan boleh/tidaknya masuk untuk ditampilkan
func isiHasilPeriksa(r *http.Request, db *sql.DB, data *dataPeriksa, kode string, eventID int64) {
	t, err := periksaKode(r.Context(), db, kode)
	var salah KesalahanInput
	switch {
	case errors.As(err, &salah):
		data.Alasan = salah.Pesan // kode tidak ditemukan, atau kode lama hasil pemindahan
		return
	case err != nil:
		log.Printf("periksa kode: %v", err)
		data.Alasan = "Terjadi kesalahan saat memeriksa kode, coba lagi."
		return
	}

	data.Ada = true
	data.Tiket = t
	if err := CekMasuk(t, eventID, time.Now()); err != nil {
		data.Alasan = err.Error()
	} else {
		data.BolehMasuk = true
	}
}

// masukHandler (BR-25, BR-26): tombol "Izinkan Masuk". Semua syarat diperiksa ulang di dalam transaksi.
func masukHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, ok := eventDariPath(w, r, db, tpl)
		if !ok {
			return
		}
		u, _ := auth.PenggunaDari(r)
		kode := r.PostFormValue("kode")

		t, err := CheckIn(r.Context(), db, e.ID, kode, u.ID, time.Now())

		var salah KesalahanInput
		switch {
		case err == nil:
			http.Redirect(w, r, fmt.Sprintf("/petugas/event/%d?berhasil=%s", e.ID, url.QueryEscape(t.Kode)), http.StatusSeeOther)
		case errors.As(err, &salah):
			// tampilkan alasan penolakan (mis. tiket baru saja dipakai di pintu lain)
			data := dataPeriksa{Halaman: auth.Halaman(r), Event: e, Kode: kode, Alasan: salah.Pesan}
			if t.ID > 0 {
				data.Ada = true
				data.Tiket = t
			}
			web.RenderStatus(w, tpl.periksa, data, http.StatusBadRequest)
		default:
			gagalServer(w, "check-in", err)
		}
	}
}

// eventDariPath membaca {id} dari URL lalu mengambil event-nya; menampilkan 400/404 jika tidak valid
func eventDariPath(w http.ResponseWriter, r *http.Request, db *sql.DB, tpl halaman) (event.Event, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		tampilPesan(w, r, tpl, http.StatusBadRequest, "Alamat tidak valid", "ID pada alamat halaman harus berupa angka.")
		return event.Event{}, false
	}
	e, err := event.GetEventById(r.Context(), db, id)
	if errors.Is(err, event.ErrTidakDitemukan) {
		tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Event tidak ditemukan.")
		return e, false
	}
	if err != nil {
		gagalServer(w, "ambil event", err)
		return e, false
	}
	return e, true
}

func tampilPesan(w http.ResponseWriter, r *http.Request, tpl halaman, status int, judul, isi string) {
	web.RenderStatus(w, tpl.pesan, web.DataPesan{Halaman: auth.Halaman(r), Judul: judul, Isi: isi, Kembali: "/petugas"}, status)
}

func gagalServer(w http.ResponseWriter, aksi string, err error) {
	log.Printf("%s: %v", aksi, err)
	http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
}
