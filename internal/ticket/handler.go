package ticket

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// dataPindah adalah data untuk templates/ticket/pindah.html
type dataPindah struct {
	web.Halaman
	Tiket    TiketPindah
	NamaBaru string // isian, supaya tetap terisi jika form gagal
}

// RegisterRoutes mendaftarkan pembatalan tiket dan pemindahan tiket (pembeli),
// serta pembatalan event (admin)
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tplPesan := web.ParseHalaman("pesan.html")
	tplPindah := web.ParseHalaman("ticket/pindah.html")

	pembeli := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.Wajib(h, auth.PeranPembeli)
	}

	mux.HandleFunc("POST /tiket/{id}/batal", pembeli(batalTiketHandler(db, tplPesan)))
	mux.HandleFunc("GET /tiket/{id}/pindah", pembeli(pindahFormHandler(db, tplPindah, tplPesan)))
	mux.HandleFunc("POST /tiket/{id}/pindah", pembeli(pindahHandler(db, tplPindah, tplPesan)))
	mux.HandleFunc("POST /admin/event/{id}/batal", auth.Wajib(batalEventHandler(db, tplPesan), auth.PeranAdmin))
}

// pindahFormHandler menampilkan form pemindahan untuk tiket milik pembeli yang login
func pindahFormHandler(db *sql.DB, tplPindah, tplPesan *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tplPesan)
		if !ok {
			return
		}
		tampilFormPindah(w, r, db, tplPindah, tplPesan, id, "", "", http.StatusOK)
	}
}

// pindahHandler (BR-21, BR-22, BR-23): mengganti nama pemegang dan membuat kode tiket baru
func pindahHandler(db *sql.DB, tplPindah, tplPesan *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tplPesan)
		if !ok {
			return
		}
		u, _ := auth.PenggunaDari(r)
		namaBaru := r.PostFormValue("nama_baru")

		hasil, err := pindahkanTiket(r.Context(), db, id, u.ID, namaBaru, time.Now())

		var salah KesalahanInput
		switch {
		case err == nil:
			http.Redirect(w, r, fmt.Sprintf("/pesanan/%d", hasil.OrderID), http.StatusSeeOther)
		case errors.Is(err, ErrTidakDitemukan):
			tampilPesan(w, r, tplPesan, http.StatusNotFound, "Tidak ditemukan", "Tiket tidak ditemukan.", "/pesanan")
		case errors.As(err, &salah):
			// tampilkan form lagi dengan alasan penolakan dan nama yang sudah diketik
			tampilFormPindah(w, r, db, tplPindah, tplPesan, id, namaBaru, salah.Pesan, http.StatusBadRequest)
		default:
			gagalServer(w, "pindah tiket", err)
		}
	}
}

func tampilFormPindah(w http.ResponseWriter, r *http.Request, db *sql.DB, tplPindah, tplPesan *template.Template, id int64, namaBaru, pesanError string, status int) {
	u, _ := auth.PenggunaDari(r)
	t, err := ambilTiketMilik(r.Context(), db, id, u.ID)
	if errors.Is(err, ErrTidakDitemukan) {
		tampilPesan(w, r, tplPesan, http.StatusNotFound, "Tidak ditemukan", "Tiket tidak ditemukan.", "/pesanan")
		return
	}
	if err != nil {
		gagalServer(w, "ambil tiket", err)
		return
	}
	data := dataPindah{Halaman: auth.Halaman(r), Tiket: t, NamaBaru: namaBaru}
	data.Error = pesanError
	web.RenderStatus(w, tplPindah, data, status)
}

// batalTiketHandler (BR-17, BR-18, BR-19): pembeli membatalkan satu tiket miliknya
func batalTiketHandler(db *sql.DB, tplPesan *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tplPesan)
		if !ok {
			return
		}
		u, _ := auth.PenggunaDari(r)

		hasil, err := batalkanTiket(r.Context(), db, id, u.ID, time.Now())

		kembali := "/pesanan"
		if hasil.OrderID > 0 {
			kembali = fmt.Sprintf("/pesanan/%d", hasil.OrderID)
		}
		var salah KesalahanInput
		switch {
		case err == nil:
			http.Redirect(w, r, kembali, http.StatusSeeOther)
		case errors.Is(err, ErrTidakDitemukan):
			tampilPesan(w, r, tplPesan, http.StatusNotFound, "Tidak ditemukan", "Tiket tidak ditemukan.", "/pesanan")
		case errors.As(err, &salah):
			tampilPesan(w, r, tplPesan, http.StatusBadRequest, "Tiket tidak bisa dibatalkan", salah.Pesan, kembali)
		default:
			gagalServer(w, "batal tiket", err)
		}
	}
}

// batalEventHandler (BR-20): admin membatalkan event OPEN yang belum dimulai
func batalEventHandler(db *sql.DB, tplPesan *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tplPesan)
		if !ok {
			return
		}
		kembali := fmt.Sprintf("/admin/event/%d", id)

		_, err := batalkanEvent(r.Context(), db, id, time.Now())

		var salah KesalahanInput
		switch {
		case err == nil:
			// ringkasan refund ditampilkan di halaman detail event
			http.Redirect(w, r, kembali, http.StatusSeeOther)
		case errors.Is(err, ErrTidakDitemukan):
			tampilPesan(w, r, tplPesan, http.StatusNotFound, "Tidak ditemukan", "Event tidak ditemukan.", "/admin/event")
		case errors.As(err, &salah):
			tampilPesan(w, r, tplPesan, http.StatusBadRequest, "Event tidak bisa dibatalkan", salah.Pesan, kembali)
		default:
			gagalServer(w, "batal event", err)
		}
	}
}

func tampilPesan(w http.ResponseWriter, r *http.Request, tpl *template.Template, status int, judul, isi, kembali string) {
	web.RenderStatus(w, tpl, web.DataPesan{Halaman: auth.Halaman(r), Judul: judul, Isi: isi, Kembali: kembali}, status)
}

func gagalServer(w http.ResponseWriter, aksi string, err error) {
	log.Printf("%s: %v", aksi, err)
	http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
}

// idDariPath membaca {id} dari URL. Jika bukan angka positif, pengguna mendapat halaman 400.
func idDariPath(w http.ResponseWriter, r *http.Request, tpl *template.Template) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		tampilPesan(w, r, tpl, http.StatusBadRequest, "Alamat tidak valid", "ID pada alamat halaman harus berupa angka.", "")
		return 0, false
	}
	return id, true
}
