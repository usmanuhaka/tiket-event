package report

import (
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// halaman mengumpulkan template fitur laporan
type halaman struct {
	list   *template.Template
	detail *template.Template
	pesan  *template.Template
}

// dataList adalah data untuk templates/report/list.html
type dataList struct {
	web.Halaman
	Event []RingkasanEvent
}

// dataDetail adalah data untuk templates/report/detail.html
type dataDetail struct {
	web.Halaman
	Laporan Laporan
	Dibaca  time.Time // kapan laporan dibaca (snapshot)
}

// RegisterRoutes mendaftarkan halaman laporan. Semuanya khusus ADMIN.
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tpl := halaman{
		list:   web.ParseHalaman("report/list.html"),
		detail: web.ParseHalaman("report/detail.html"),
		pesan:  web.ParseHalaman("pesan.html"),
	}

	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.Wajib(h, auth.PeranAdmin)
	}

	mux.HandleFunc("GET /admin/laporan", admin(laporanListHandler(db, tpl)))
	mux.HandleFunc("GET /admin/laporan/{id}", admin(laporanDetailHandler(db, tpl)))
}

func laporanListHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		daftar, err := ambilRingkasanSemuaEvent(r.Context(), db)
		if err != nil {
			gagalServer(w, "ringkasan laporan", err)
			return
		}
		web.Render(w, tpl.list, dataList{Halaman: auth.Halaman(r), Event: daftar})
	}
}

// laporanDetailHandler (BR-27, BR-28): laporan per tahap, per kategori, dan total event
func laporanDetailHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			tampilPesan(w, r, tpl, http.StatusBadRequest, "Alamat tidak valid", "ID pada alamat halaman harus berupa angka.")
			return
		}

		dibaca := time.Now()
		lap, err := AmbilLaporan(r.Context(), db, id)
		if errors.Is(err, ErrTidakDitemukan) {
			tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Event tidak ditemukan.")
			return
		}
		if err != nil {
			gagalServer(w, "laporan event", err)
			return
		}
		web.Render(w, tpl.detail, dataDetail{Halaman: auth.Halaman(r), Laporan: lap, Dibaca: dibaca})
	}
}

func tampilPesan(w http.ResponseWriter, r *http.Request, tpl halaman, status int, judul, isi string) {
	web.RenderStatus(w, tpl.pesan, web.DataPesan{Halaman: auth.Halaman(r), Judul: judul, Isi: isi, Kembali: "/admin/laporan"}, status)
}

func gagalServer(w http.ResponseWriter, aksi string, err error) {
	log.Printf("%s: %v", aksi, err)
	http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
}
