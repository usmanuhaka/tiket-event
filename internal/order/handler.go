package order

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
	"github.com/usmanuhaka/tiket-event/internal/event"
	"github.com/usmanuhaka/tiket-event/internal/ticket"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

// halaman mengumpulkan semua template fitur pemesanan
type halaman struct {
	katalog *template.Template
	event   *template.Template
	list    *template.Template
	detail  *template.Template
	pesan   *template.Template
}

// dataKatalog adalah data untuk templates/order/katalog.html
type dataKatalog struct {
	web.Halaman
	Event []event.Event
}

// kategoriPilihan adalah satu kategori di halaman event pembeli
type kategoriPilihan struct {
	ID       int64
	Nama     string
	Aktif    event.Tahap // tahap yang harganya berlaku sekarang (BR-05)
	AdaAktif bool        // false berarti kategori habis
	Sisa     int         // tiket yang masih bisa dipesan
}

// isianBaris menyimpan isian satu baris form pesan, supaya form yang gagal tetap terisi
type isianBaris struct {
	KategoriID int64
	Nama       string
}

// dataEvent adalah data untuk templates/order/event.html
type dataEvent struct {
	web.Halaman
	Event     event.Event
	Kategori  []kategoriPilihan
	Isian     []isianBaris // selalu 4 baris (BR-06: maksimal 4 tiket)
	KodePromo string       // isian kode promo, supaya tetap terisi jika form gagal
}

// dataPesananList adalah data untuk templates/order/list.html
type dataPesananList struct {
	web.Halaman
	Pesanan []Pesanan
}

// dataPesanan adalah data untuk templates/order/detail.html
type dataPesanan struct {
	web.Halaman
	Pesanan    Pesanan
	BisaBayar  bool      // HELD dan belum lewat 15 menit
	BatasBatal time.Time // tanggal terakhir pembatalan tiket (BR-17)
}

// RegisterRoutes mendaftarkan halaman katalog event, pemesanan, dan pembayaran
func RegisterRoutes(mux *http.ServeMux, db *sql.DB) {
	tpl := halaman{
		katalog: web.ParseHalaman("order/katalog.html"),
		event:   web.ParseHalaman("order/event.html"),
		list:    web.ParseHalaman("order/list.html"),
		detail:  web.ParseHalaman("order/detail.html"),
		pesan:   web.ParseHalaman("pesan.html"),
	}

	// pembeli membungkus handler supaya hanya bisa dipakai peran PEMBELI
	pembeli := func(h http.HandlerFunc) http.HandlerFunc {
		return auth.Wajib(h, auth.PeranPembeli)
	}

	// katalog bisa dilihat siapa saja, termasuk tamu
	mux.HandleFunc("GET /event", katalogHandler(db, tpl))
	mux.HandleFunc("GET /event/{id}", eventHandler(db, tpl))

	mux.HandleFunc("POST /event/{id}/pesan", pembeli(pesanHandler(db, tpl)))
	mux.HandleFunc("GET /pesanan", pembeli(pesananListHandler(db, tpl)))
	mux.HandleFunc("GET /pesanan/{id}", pembeli(pesananDetailHandler(db, tpl)))
	mux.HandleFunc("POST /pesanan/{id}/bayar", pembeli(bayarHandler(db, tpl)))
}

// ===== Katalog event =====

func katalogHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		daftar, err := event.AmbilEventDibuka(r.Context(), db, time.Now())
		if err != nil {
			gagalServer(w, "ambil katalog", err)
			return
		}
		web.Render(w, tpl.katalog, dataKatalog{Halaman: auth.Halaman(r), Event: daftar})
	}
}

func eventHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl)
		if !ok {
			return
		}
		tampilEvent(w, r, db, tpl, id, make([]isianBaris, MaksTiket), "", "", http.StatusOK)
	}
}

// tampilEvent merender halaman event pembeli: harga tahap aktif tiap kategori dan form pesan
func tampilEvent(w http.ResponseWriter, r *http.Request, db *sql.DB, tpl halaman, id int64, isian []isianBaris, kodePromo, pesanError string, status int) {
	now := time.Now()

	e, err := event.GetEventById(r.Context(), db, id)
	if errors.Is(err, event.ErrTidakDitemukan) || (err == nil && (e.Status != event.StatusOpen || !e.TanggalEvent.After(now))) {
		// event DRAFT, dibatalkan, atau sudah lewat tidak ditampilkan ke pembeli
		tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Event tidak ditemukan atau tidak sedang dijual.")
		return
	}
	if err != nil {
		gagalServer(w, "ambil event", err)
		return
	}

	kategori, err := event.AmbilKategoriDanTahap(r.Context(), db, id)
	if err != nil {
		gagalServer(w, "ambil kategori", err)
		return
	}

	data := dataEvent{Halaman: auth.Halaman(r), Event: e, Isian: isian, KodePromo: kodePromo}
	data.Error = pesanError
	for _, k := range kategori {
		aktif, ada := event.TahapAktif(k.Tahap, now) // BR-05
		data.Kategori = append(data.Kategori, kategoriPilihan{
			ID:       k.ID,
			Nama:     k.Nama,
			Aktif:    aktif,
			AdaAktif: ada,
			Sisa:     event.SisaTersedia(k.Tahap, now),
		})
	}
	web.RenderStatus(w, tpl.event, data, status)
}

// ===== Pesan (BR-06 s.d. BR-09) =====

func pesanHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl)
		if !ok {
			return
		}
		u, _ := auth.PenggunaDari(r) // pasti ada karena dibungkus auth.Wajib

		baris, isian, err := bacaFormPesan(r)
		kodePromo := r.PostFormValue("kode_promo") // boleh kosong (BR-13 s.d. BR-16 dicek di buatPesanan)
		if err == nil {
			// kuota dikurangi dan pesanan HELD dibuat di dalam satu transaksi
			var orderID int64
			orderID, err = buatPesanan(r.Context(), db, u.ID, id, kodePromo, baris, time.Now())
			if err == nil {
				http.Redirect(w, r, fmt.Sprintf("/pesanan/%d", orderID), http.StatusSeeOther)
				return
			}
		}

		var salah KesalahanInput
		switch {
		case errors.As(err, &salah):
			tampilEvent(w, r, db, tpl, id, isian, kodePromo, salah.Pesan, http.StatusBadRequest)
		case errors.Is(err, ErrTidakDitemukan):
			tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Event tidak ditemukan atau tidak sedang dijual.")
		default:
			gagalServer(w, "buat pesanan", err)
		}
	}
}

// bacaFormPesan membaca 4 baris form (kategori_1..4 dan nama_1..4).
// Baris yang kategori dan namanya kosong diabaikan.
// isian selalu dikembalikan lengkap supaya form bisa ditampilkan lagi jika gagal.
func bacaFormPesan(r *http.Request) ([]BarisTiket, []isianBaris, error) {
	isian := make([]isianBaris, MaksTiket)
	var baris []BarisTiket
	var errPertama error

	for i := 0; i < MaksTiket; i++ {
		teksKategori := strings.TrimSpace(r.PostFormValue(fmt.Sprintf("kategori_%d", i+1)))
		nama := r.PostFormValue(fmt.Sprintf("nama_%d", i+1))
		isian[i].Nama = nama

		if teksKategori == "" && strings.TrimSpace(nama) == "" {
			continue // baris kosong
		}
		if teksKategori == "" {
			if errPertama == nil {
				errPertama = salahInput("Tiket ke-%d: pilih kategori", i+1)
			}
			continue
		}
		kategoriID, err := strconv.ParseInt(teksKategori, 10, 64)
		if err != nil {
			if errPertama == nil {
				errPertama = salahInput("Tiket ke-%d: kategori tidak valid", i+1)
			}
			continue
		}
		isian[i].KategoriID = kategoriID
		baris = append(baris, BarisTiket{KategoriID: kategoriID, NamaPemegang: nama})
	}

	if errPertama != nil {
		return nil, isian, errPertama
	}
	// BR-06 (jumlah 1-4) dan BR-07 (nama pemegang wajib)
	return baris, isian, ValidasiPesanan(baris)
}

// ===== Pesanan saya dan pembayaran =====

func pesananListHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.PenggunaDari(r)
		daftar, err := ambilPesananUser(r.Context(), db, u.ID)
		if err != nil {
			gagalServer(w, "ambil pesanan", err)
			return
		}
		web.Render(w, tpl.list, dataPesananList{Halaman: auth.Halaman(r), Pesanan: daftar})
	}
}

func pesananDetailHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl)
		if !ok {
			return
		}
		tampilPesanan(w, r, db, tpl, id, "", http.StatusOK)
	}
}

func bayarHandler(db *sql.DB, tpl halaman) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := idDariPath(w, r, tpl)
		if !ok {
			return
		}
		u, _ := auth.PenggunaDari(r)

		// BR-11 (dan BR-10: batas waktu dicek ulang di dalam bayarPesanan)
		err := bayarPesanan(r.Context(), db, id, u.ID, time.Now())

		var salah KesalahanInput
		switch {
		case err == nil:
			http.Redirect(w, r, fmt.Sprintf("/pesanan/%d", id), http.StatusSeeOther)
		case errors.Is(err, ErrTidakDitemukan):
			tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Pesanan tidak ditemukan.")
		case errors.As(err, &salah):
			tampilPesanan(w, r, db, tpl, id, salah.Pesan, http.StatusBadRequest)
		default:
			gagalServer(w, "bayar pesanan", err)
		}
	}
}

// tampilPesanan merender detail pesanan milik pembeli yang login
func tampilPesanan(w http.ResponseWriter, r *http.Request, db *sql.DB, tpl halaman, id int64, pesanError string, status int) {
	u, _ := auth.PenggunaDari(r)
	p, err := ambilPesananMilik(r.Context(), db, id, u.ID)
	if errors.Is(err, ErrTidakDitemukan) {
		tampilPesan(w, r, tpl, http.StatusNotFound, "Tidak ditemukan", "Pesanan tidak ditemukan.")
		return
	}
	if err != nil {
		gagalServer(w, "ambil pesanan", err)
		return
	}

	data := dataPesanan{Halaman: auth.Halaman(r), Pesanan: p}
	data.Error = pesanError
	if p.Status == StatusHeld {
		if SudahKedaluwarsa(p.ExpiresAt, time.Now()) {
			// worker belum sempat memproses; tombol bayar disembunyikan
			if data.Error == "" {
				data.Error = "Waktu pembayaran sudah habis. Pesanan akan segera dibatalkan otomatis."
			}
		} else {
			data.BisaBayar = true
		}
	}

	// tombol "Batalkan" per tiket: hanya tiket ACTIVE, event masih OPEN, dan belum lewat H-7 (BR-17)
	now := time.Now()
	data.BatasBatal = ticket.BatasBatal(p.TanggalEvent)
	for i, t := range data.Pesanan.Tiket {
		if t.Status == TiketActive && p.StatusEvent == event.StatusOpen && ticket.BolehBatal(p.TanggalEvent, now) {
			data.Pesanan.Tiket[i].BisaBatal = true
			data.Pesanan.Tiket[i].EstimasiRefund = ticket.HitungRefund(t.HargaBayar, ticket.PersenRefundPembeli)
		}
	}
	web.RenderStatus(w, tpl.detail, data, status)
}

// ===== Helper =====

func tampilPesan(w http.ResponseWriter, r *http.Request, tpl halaman, status int, judul, isi string) {
	web.RenderStatus(w, tpl.pesan, web.DataPesan{Halaman: auth.Halaman(r), Judul: judul, Isi: isi}, status)
}

func gagalServer(w http.ResponseWriter, aksi string, err error) {
	log.Printf("%s: %v", aksi, err)
	http.Error(w, "Terjadi kesalahan, coba lagi", http.StatusInternalServerError)
}

// idDariPath membaca {id} dari URL. Jika bukan angka positif, pengguna mendapat halaman 400.
func idDariPath(w http.ResponseWriter, r *http.Request, tpl halaman) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		tampilPesan(w, r, tpl, http.StatusBadRequest, "Alamat tidak valid", "ID pada alamat halaman harus berupa angka.")
		return 0, false
	}
	return id, true
}
