package event

import (
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// halaman mengumpulkan semua template fitur event supaya cukup dikirim sebagai satu parameter
type halaman struct {
	list   *template.Template
	form   *template.Template
	detail *template.Template
	pesan  *template.Template
}

// dataList adalah data untuk templates/event/list.html
type dataList struct {
	web.Halaman
	Event []Event
}

// dataForm adalah data untuk templates/event/form.html (tambah dan edit).
// TanggalInput dan KapasitasInput menyimpan isian apa adanya,
// supaya form yang gagal disimpan tetap terisi.
type dataForm struct {
	web.Halaman
	Judul          string
	Action         string
	Kembali        string
	Event          Event
	TanggalInput   string
	KapasitasInput string
}

// kategoriTampil adalah kategori beserta ringkasan untuk halaman detail
type kategoriTampil struct {
	Kategori
	TotalKuotaTahap int
	Aktif           Tahap
	AdaAktif        bool
}

// dataDetail adalah data untuk templates/event/detail.html
type dataDetail struct {
	web.Halaman
	Event              Event
	Kategori           []kategoriTampil
	TotalKuotaKategori int
	BisaDiubah         bool     // true selama event masih DRAFT
	Masalah            []string // alasan "Buka Penjualan" ditolak
	BisaDibatalkan     bool     // OPEN dan belum dimulai (BR-20)
	Refund             RingkasanRefund
}

// RegisterRoutes mendaftarkan semua halaman pengelolaan event. Semuanya khusus ADMIN.
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tpl := halaman{
		list:   web.ParseHalaman("event/list.html"),
		form:   web.ParseHalaman("event/form.html"),
		detail: web.ParseHalaman("event/detail.html"),
		pesan:  web.ParseHalaman("pesan.html"),
	}

	// admin membungkus handler supaya hanya bisa dibuka peran ADMIN
	admin := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.Wajib(h, auth.PeranAdmin)
	}

	mux.HandleFunc("GET /admin/event", admin(eventListHandler(db, tpl)))
	mux.HandleFunc("GET /admin/event/form", admin(eventFormHandler(tpl)))
	mux.HandleFunc("POST /admin/event", admin(eventSimpanHandler(db, tpl)))
	mux.HandleFunc("GET /admin/event/{id}", admin(eventDetailHandler(db, tpl)))

	// update dan hapus (hanya DRAFT)
	mux.HandleFunc("GET /admin/event/{id}/edit", admin(eventEditFormHandler(db, tpl)))
	mux.HandleFunc("POST /admin/event/{id}/edit", admin(eventUpdateHandler(db, tpl)))
	mux.HandleFunc("POST /admin/event/{id}/hapus", admin(eventHapusHandler(db, tpl)))

	// kategori dan tahap: cukup tambah + hapus
	mux.HandleFunc("POST /admin/event/{id}/kategori", admin(kategoriTambahHandler(db, tpl)))
	mux.HandleFunc("POST /admin/event/{id}/kategori/{kid}/hapus", admin(kategoriHapusHandler(db, tpl)))
	mux.HandleFunc("POST /admin/event/{id}/kategori/{kid}/tahap", admin(tahapTambahHandler(db, tpl)))
	mux.HandleFunc("POST /admin/event/{id}/tahap/{tid}/hapus", admin(tahapHapusHandler(db, tpl)))

	// DRAFT -> OPEN
	mux.HandleFunc("POST /admin/event/{id}/buka", admin(bukaPenjualanHandler(db, tpl)))
}

// ===== Event =====

func eventListHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		daftar, err := ambilSemuaEvent(r.Context(), db)
		if err != nil {
			gagalServer(w, "ambil daftar event", err)
			return
		}
		web.Render(w, tpl.list, dataList{Halaman: auth.Halaman(r), Event: daftar})
	}
}

func eventFormHandler(tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		web.Render(w, tpl.form, dataForm{
			Halaman: auth.Halaman(r),
			Judul:   "Tambah Event",
			Action:  "/admin/event",
			Kembali: "/admin/event",
		})
	}
}

func eventSimpanHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := dataForm{
			Halaman: auth.Halaman(r),
			Judul:   "Tambah Event",
			Action:  "/admin/event",
			Kembali: "/admin/event",
		}

		e, err := bacaFormEvent(r, &data)
		if err == nil {
			err = ValidasiEvent(e, time.Now())
		}
		if err != nil {
			data.Error = err.Error()
			web.RenderStatus(w, tpl.form, data, http.StatusBadRequest)
			return
		}

		id, err := simpanEvent(r.Context(), db, e)
		if err != nil {
			gagalServer(w, "simpan event", err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
	}
}

func eventDetailHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}
		tampilDetail(w, r, db, tpl, id, "", nil, http.StatusOK)
	}
}

func eventEditFormHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}

		e, err := GetEventById(r.Context(), db, id)
		if err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		if e.Status != StatusDraft {
			tanggapiError(w, r, db, tpl, id, ErrBukanDraft)
			return
		}

		web.Render(w, tpl.form, dataForm{
			Halaman:        auth.Halaman(r),
			Judul:          "Edit Event",
			Action:         fmt.Sprintf("/admin/event/%d/edit", id),
			Kembali:        fmt.Sprintf("/admin/event/%d", id),
			Event:          e,
			TanggalInput:   web.WaktuForm(e.TanggalEvent),
			KapasitasInput: strconv.Itoa(e.Kapasitas),
		})
	}
}

func eventUpdateHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}
		data := dataForm{
			Halaman: auth.Halaman(r),
			Judul:   "Edit Event",
			Action:  fmt.Sprintf("/admin/event/%d/edit", id),
			Kembali: fmt.Sprintf("/admin/event/%d", id),
		}

		e, err := bacaFormEvent(r, &data)
		e.ID = id
		if err == nil {
			err = ValidasiEvent(e, time.Now())
		}
		if err == nil {
			err = updateEvent(r.Context(), db, e)
		}

		var salah KesalahanInput
		switch {
		case err == nil:
			http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
		case errors.Is(err, ErrBukanDraft) || errors.Is(err, ErrTidakDitemukan):
			tanggapiError(w, r, db, tpl, id, err)
		case errors.As(err, &salah):
			// isi form salah: tampilkan form lagi dengan isian yang sama
			data.Error = salah.Pesan
			web.RenderStatus(w, tpl.form, data, http.StatusBadRequest)
		default:
			gagalServer(w, "update event", err)
		}
	}
}

func eventHapusHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}
		if err := hapusEvent(r.Context(), db, id); err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		http.Redirect(w, r, "/admin/event", http.StatusSeeOther)
	}
}

// ===== Kategori dan tahap =====

func kategoriTambahHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}

		nama := strings.TrimSpace(r.PostFormValue("nama"))
		kuota, err := strconv.Atoi(r.PostFormValue("kuota"))
		if err != nil {
			err = salahInput("Kuota kategori harus berupa angka bulat")
		} else {
			err = ValidasiKategori(nama, kuota)
		}
		if err == nil {
			// BR-01 dan BR-02 dicek di dalam transaksi
			_, err = tambahKategori(r.Context(), db, id, nama, kuota)
		}
		if err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
	}
}

func kategoriHapusHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}
		kategoriID, ok := idDariPath(w, r, tpl, "kid")
		if !ok {
			return
		}
		if err := hapusKategori(r.Context(), db, id, kategoriID); err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
	}
}

func tahapTambahHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}
		kategoriID, ok := idDariPath(w, r, tpl, "kid")
		if !ok {
			return
		}

		t, err := bacaFormTahap(r)
		if err == nil {
			err = ValidasiTahap(t)
		}
		if err == nil {
			// BR-03 (urutan otomatis) dan BR-04 (kuota tahap) dicek di dalam transaksi
			_, err = tambahTahap(r.Context(), db, id, kategoriID, t)
		}
		if err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
	}
}

func tahapHapusHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}
		tahapID, ok := idDariPath(w, r, tpl, "tid")
		if !ok {
			return
		}
		if err := hapusTahap(r.Context(), db, id, tahapID); err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
	}
}

// ===== Buka penjualan =====

func bukaPenjualanHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl, "id")
		if !ok {
			return
		}

		masalah, err := bukaPenjualan(r.Context(), db, id, time.Now())
		if err != nil {
			tanggapiError(w, r, db, tpl, id, err)
			return
		}
		if len(masalah) > 0 {
			// syarat belum terpenuhi: tampilkan semua masalah, status tetap DRAFT
			tampilDetail(w, r, db, tpl, id, "", masalah, http.StatusBadRequest)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/admin/event/%d", id), http.StatusSeeOther)
	}
}

// ===== Helper =====

// tampilDetail merender halaman detail event. Dipakai saat membuka halaman (200)
// dan saat sebuah aksi ditolak (400) supaya admin melihat pesan di halaman yang sama.
func tampilDetail(w http.ResponseWriter, r *http.Request, db *sql.DB, tpl halaman, id int64, pesanError string, masalah []string, status int) {
	e, err := GetEventById(r.Context(), db, id)
	if errors.Is(err, ErrTidakDitemukan) {
		tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Event tidak ditemukan atau sudah dihapus.")
		return
	}
	if err != nil {
		gagalServer(w, "ambil event", err)
		return
	}

	kategori, err := AmbilKategoriDanTahap(r.Context(), db, id)
	if err != nil {
		gagalServer(w, "ambil kategori", err)
		return
	}

	data := dataDetail{
		Halaman:    auth.Halaman(r),
		Event:      e,
		BisaDiubah: e.Status == StatusDraft,
		Masalah:    masalah,
		// BR-20: hanya event OPEN yang belum dimulai yang bisa dibatalkan
		BisaDibatalkan: e.Status == StatusOpen && e.TanggalEvent.After(time.Now()),
	}
	data.Error = pesanError

	if e.Status == StatusCancelled {
		data.Refund, err = ringkasanRefundEvent(r.Context(), db, id)
		if err != nil {
			gagalServer(w, "ringkasan refund", err)
			return
		}
	}

	now := time.Now()
	for _, k := range kategori {
		kt := kategoriTampil{Kategori: k}
		for _, t := range k.Tahap {
			kt.TotalKuotaTahap += t.Kuota
		}
		// BR-05: tahap yang harganya berlaku saat ini
		kt.Aktif, kt.AdaAktif = TahapAktif(k.Tahap, now)
		// tahap berkuota 0 (kuotanya sudah dipindah) tidak ditampilkan
		kt.Tahap = TahapTerlihat(k.Tahap)

		data.Kategori = append(data.Kategori, kt)
		data.TotalKuotaKategori += k.Kuota
	}

	web.RenderStatus(w, tpl.detail, data, status)
}

// tanggapiError menampilkan error sebuah aksi sesuai jenisnya:
// data tidak ada -> 404, kesalahan input/aturan bisnis -> halaman detail dengan pesan (400),
// selain itu -> 500 (detail error hanya dicatat di log, tidak ditampilkan ke pengguna).
func tanggapiError(w http.ResponseWriter, r *http.Request, db *sql.DB, tpl halaman, eventID int64, err error) {
	var salah KesalahanInput
	switch {
	case errors.Is(err, ErrTidakDitemukan):
		tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Event, kategori, atau tahap tidak ditemukan atau sudah dihapus.")
	case errors.As(err, &salah):
		tampilDetail(w, r, db, tpl, eventID, salah.Pesan, nil, http.StatusBadRequest)
	default:
		gagalServer(w, "aksi event", err)
	}
}

func tampilPesan(w http.ResponseWriter, r *http.Request, tpl halaman, status int, judul, isi string) {
	web.RenderStatus(w, tpl.pesan, web.DataPesan{Halaman: auth.Halaman(r), Judul: judul, Isi: isi}, status)
}

func gagalServer(w http.ResponseWriter, aksi string, err error) {
	log.Printf("%s: %v", aksi, err)
	http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
}

// idDariPath membaca angka id dari URL (mis. {id} pada /admin/event/{id}).
// Jika bukan angka positif, pengguna mendapat halaman 400 dan ok bernilai false.
func idDariPath(w http.ResponseWriter, r *http.Request, tpl halaman, nama string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(nama), 10, 64)
	if err != nil || id <= 0 {
		tampilPesan(w, r, tpl, http.StatusBadRequest, "Alamat tidak valid", "ID pada alamat halaman harus berupa angka.")
		return 0, false
	}
	return id, true
}

// bacaFormEvent membaca isian form event. Isian mentah disalin ke data
// supaya form yang gagal tetap terisi seperti yang diketik admin.
func bacaFormEvent(r *http.Request, data *dataForm) (Event, error) {
	e := Event{
		Nama:      strings.TrimSpace(r.PostFormValue("nama")),
		Lokasi:    strings.TrimSpace(r.PostFormValue("lokasi")),
		Deskripsi: strings.TrimSpace(r.PostFormValue("deskripsi")),
	}
	data.Event = e
	data.TanggalInput = r.PostFormValue("tanggal_event")
	data.KapasitasInput = r.PostFormValue("kapasitas")

	kapasitas, err := strconv.Atoi(data.KapasitasInput)
	if err != nil {
		return e, salahInput("Kapasitas harus berupa angka bulat")
	}
	e.Kapasitas = kapasitas

	tanggal, err := web.ParseWaktuForm(data.TanggalInput)
	if err != nil {
		return e, salahInput("Tanggal dan jam event tidak valid")
	}
	e.TanggalEvent = tanggal

	data.Event = e
	return e, nil
}

// bacaFormTahap membaca isian form tambah tahap
func bacaFormTahap(r *http.Request) (Tahap, error) {
	t := Tahap{Nama: strings.TrimSpace(r.PostFormValue("nama"))}

	harga, err := strconv.ParseInt(r.PostFormValue("harga"), 10, 64)
	if err != nil {
		return t, salahInput("Harga harus berupa angka bulat (tanpa Rp dan titik)")
	}
	t.Harga = harga

	kuota, err := strconv.Atoi(r.PostFormValue("kuota"))
	if err != nil {
		return t, salahInput("Kuota tahap harus berupa angka bulat")
	}
	t.Kuota = kuota

	batas, err := web.ParseWaktuForm(r.PostFormValue("batas_tanggal"))
	if err != nil {
		return t, salahInput("Batas tanggal tahap tidak valid")
	}
	t.BatasTanggal = batas

	return t, nil
}
