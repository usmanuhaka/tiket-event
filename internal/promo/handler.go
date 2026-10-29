package promo

import (
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// halaman mengumpulkan template fitur promo
type halaman struct {
	list  *template.Template
	form  *template.Template
	pesan *template.Template
}

// dataList adalah data untuk templates/promo/list.html
type dataList struct {
	web.Halaman
	Promo []Promo
}

// dataForm adalah data untuk templates/promo/form.html.
// Isian disimpan apa adanya supaya form yang gagal tetap terisi.
type dataForm struct {
	web.Halaman
	Pilihan     []PilihanKategori
	Dipilih     map[int64]bool // kategori yang dicentang
	KodeInput   string
	PersenInput string
	KuotaInput  string
}

// RegisterRoutes mendaftarkan halaman pengelolaan promo. Semuanya khusus ADMIN.
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tpl := halaman{
		list:  web.ParseHalaman("promo/list.html"),
		form:  web.ParseHalaman("promo/form.html"),
		pesan: web.ParseHalaman("pesan.html"),
	}

	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.Wajib(h, auth.PeranAdmin)
	}

	mux.HandleFunc("GET /admin/promo", admin(promoListHandler(db, tpl)))
	mux.HandleFunc("GET /admin/promo/form", admin(promoFormHandler(db, tpl)))
	mux.HandleFunc("POST /admin/promo", admin(promoSimpanHandler(db, tpl)))
	mux.HandleFunc("POST /admin/promo/{id}/hapus", admin(promoHapusHandler(db, tpl)))
}

func promoListHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		daftar, err := ambilSemuaPromo(r.Context(), db)
		if err != nil {
			gagalServer(w, "ambil promo", err)
			return
		}
		web.Render(w, tpl.list, dataList{Halaman: auth.Halaman(r), Promo: daftar})
	}
}

func promoFormHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tampilForm(w, r, db, tpl, dataForm{Dipilih: map[int64]bool{}}, http.StatusOK)
	}
}

func promoSimpanHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ParseForm perlu dipanggil supaya r.PostForm["kategori"] (banyak kotak centang) terisi
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Isi form tidak bisa dibaca", http.StatusBadRequest)
			return
		}

		data := dataForm{
			Dipilih:     map[int64]bool{},
			KodeInput:   r.PostFormValue("kode"),
			PersenInput: r.PostFormValue("persen"),
			KuotaInput:  r.PostFormValue("kuota"),
		}
		p, err := bacaFormPromo(r, &data)
		if err == nil {
			err = ValidasiPromo(p) // BR-13
		}
		if err == nil {
			_, err = simpanPromo(r.Context(), db, p)
		}

		var salah KesalahanInput
		switch {
		case err == nil:
			http.Redirect(w, r, "/admin/promo", http.StatusSeeOther)
		case errors.As(err, &salah):
			data.Error = salah.Pesan
			tampilForm(w, r, db, tpl, data, http.StatusBadRequest)
		default:
			gagalServer(w, "simpan promo", err)
		}
	}
}

func promoHapusHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			tampilPesan(w, r, tpl, http.StatusBadRequest, "Alamat tidak valid", "ID pada alamat halaman harus berupa angka.")
			return
		}

		err = hapusPromo(r.Context(), db, id)
		switch {
		case err == nil:
			http.Redirect(w, r, "/admin/promo", http.StatusSeeOther)
		case errors.Is(err, ErrTidakDitemukan):
			tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Promo tidak ditemukan atau sudah dihapus.")
		default:
			gagalServer(w, "hapus promo", err)
		}
	}
}

// bacaFormPromo membaca isian form. Kotak centang yang dipilih dicatat di data.Dipilih.
func bacaFormPromo(r *http.Request, data *dataForm) (Promo, error) {
	p := Promo{Kode: NormalisasiKode(data.KodeInput)}

	for _, teks := range r.PostForm["kategori"] {
		id, err := strconv.ParseInt(teks, 10, 64)
		if err != nil {
			return p, salahInput("Pilihan kategori tidak valid")
		}
		p.KategoriID = append(p.KategoriID, id)
		data.Dipilih[id] = true
	}

	persen, err := strconv.Atoi(data.PersenInput)
	if err != nil {
		return p, salahInput("Potongan harus berupa angka bulat")
	}
	p.Persen = persen

	kuota, err := strconv.Atoi(data.KuotaInput)
	if err != nil {
		return p, salahInput("Kuota pemakaian harus berupa angka bulat")
	}
	p.KuotaPemakaian = kuota

	return p, nil
}

// tampilForm merender form promo beserta daftar kategori yang bisa dicentang
func tampilForm(w http.ResponseWriter, r *http.Request, db *sql.DB, tpl halaman, data dataForm, status int) {
	pilihan, err := ambilPilihanKategori(r.Context(), db)
	if err != nil {
		gagalServer(w, "ambil kategori", err)
		return
	}
	pesanError := data.Error
	data.Halaman = auth.Halaman(r)
	data.Error = pesanError
	data.Pilihan = pilihan
	web.RenderStatus(w, tpl.form, data, status)
}

func tampilPesan(w http.ResponseWriter, r *http.Request, tpl halaman, status int, judul, isi string) {
	web.RenderStatus(w, tpl.pesan, web.DataPesan{Halaman: auth.Halaman(r), Judul: judul, Isi: isi}, status)
}

func gagalServer(w http.ResponseWriter, aksi string, err error) {
	log.Printf("%s: %v", aksi, err)
	http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
}
