// Package event mengurus event, kategori tiket, dan tahap harga (BR-01 s.d. BR-05).
package event

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Status event, sama dengan nilai ENUM kolom events.status
const (
	StatusDraft     = "DRAFT"
	StatusOpen      = "OPEN"
	StatusCancelled = "CANCELLED"
)

// Event adalah satu baris tabel events
type Event struct {
	ID           int64
	Nama         string
	Lokasi       string
	Deskripsi    string
	TanggalEvent time.Time
	Kapasitas    int
	Status       string
}

// Kategori adalah satu baris tabel ticket_categories beserta tahap-tahapnya
type Kategori struct {
	ID      int64
	EventID int64
	Nama    string
	Kuota   int
	Tahap   []Tahap
}

// Tahap adalah satu baris tabel price_tiers
type Tahap struct {
	ID           int64
	KategoriID   int64
	Nama         string
	Urutan       int
	Harga        int64
	Kuota        int
	SisaKuota    int
	BatasTanggal time.Time
}

// KesalahanInput adalah kesalahan karena isi form atau aturan bisnis, bukan karena server rusak.
// Handler menampilkan pesannya ke admin dengan status 400.
type KesalahanInput struct {
	Pesan string
}

func (k KesalahanInput) Error() string {
	return k.Pesan
}

func salahInput(format string, a ...any) error {
	return KesalahanInput{Pesan: fmt.Sprintf(format, a...)}
}

// ErrBukanDraft dikembalikan jika admin mengubah event yang penjualannya sudah dibuka
var ErrBukanDraft = KesalahanInput{Pesan: "Event sudah dibuka, data tidak bisa diubah"}

// ===== Validasi isi form =====

// ValidasiEvent memeriksa isi form event.
// Tanggal event boleh hari ini, asalkan jamnya belum lewat (berguna untuk demo check-in).
func ValidasiEvent(e Event, now time.Time) error {
	if strings.TrimSpace(e.Nama) == "" {
		return salahInput("Nama event wajib diisi")
	}
	if utf8.RuneCountInString(e.Nama) > 150 {
		return salahInput("Nama event maksimal 150 karakter")
	}
	if strings.TrimSpace(e.Lokasi) == "" {
		return salahInput("Lokasi wajib diisi")
	}
	if utf8.RuneCountInString(e.Lokasi) > 150 {
		return salahInput("Lokasi maksimal 150 karakter")
	}
	if e.Kapasitas <= 0 {
		return salahInput("Kapasitas harus lebih dari 0")
	}
	if !e.TanggalEvent.After(now) {
		return salahInput("Tanggal dan jam event sudah lewat")
	}
	return nil
}

// ValidasiKategori memeriksa isi form kategori
func ValidasiKategori(nama string, kuota int) error {
	if strings.TrimSpace(nama) == "" {
		return salahInput("Nama kategori wajib diisi")
	}
	if utf8.RuneCountInString(nama) > 100 {
		return salahInput("Nama kategori maksimal 100 karakter")
	}
	if kuota <= 0 {
		return salahInput("Kuota kategori harus lebih dari 0")
	}
	return nil
}

// ValidasiTahap memeriksa isi form tahap harga
func ValidasiTahap(t Tahap) error {
	if strings.TrimSpace(t.Nama) == "" {
		return salahInput("Nama tahap wajib diisi")
	}
	if utf8.RuneCountInString(t.Nama) > 100 {
		return salahInput("Nama tahap maksimal 100 karakter")
	}
	if t.Harga < 0 {
		return salahInput("Harga tidak boleh minus")
	}
	if t.Kuota <= 0 {
		return salahInput("Kuota tahap harus lebih dari 0")
	}
	if t.BatasTanggal.IsZero() {
		return salahInput("Batas tanggal tahap wajib diisi")
	}
	return nil
}

// ===== Aturan kuota =====

// CekKuotaKategori (BR-02): total kuota semua kategori tidak boleh melebihi kapasitas event.
// totalSekarang adalah jumlah kuota kategori yang sudah ada, kuotaBaru kategori yang akan ditambah.
func CekKuotaKategori(kapasitas, totalSekarang, kuotaBaru int) error {
	if totalSekarang+kuotaBaru > kapasitas {
		return salahInput("Total kuota kategori menjadi %d, melebihi kapasitas event %d (sisa yang bisa dibagi: %d)",
			totalSekarang+kuotaBaru, kapasitas, kapasitas-totalSekarang)
	}
	return nil
}

// CekKapasitasBaru (BR-02 saat edit event): kapasitas tidak boleh diturunkan
// di bawah total kuota kategori yang sudah dibuat.
func CekKapasitasBaru(kapasitasBaru, totalKuotaKategori int) error {
	if kapasitasBaru < totalKuotaKategori {
		return salahInput("Kapasitas %d lebih kecil dari total kuota kategori yang sudah ada (%d)",
			kapasitasBaru, totalKuotaKategori)
	}
	return nil
}

// CekKuotaTahap (BR-04 saat menambah tahap): total kuota tahap tidak boleh melebihi kuota kategori.
// Syarat "harus sama persis" baru dicek saat penjualan dibuka (ValidasiBukaPenjualan),
// karena admin menambah tahap satu per satu.
func CekKuotaTahap(kuotaKategori, totalSekarang, kuotaBaru int) error {
	if totalSekarang+kuotaBaru > kuotaKategori {
		return salahInput("Total kuota tahap menjadi %d, melebihi kuota kategori %d (sisa yang bisa dibagi: %d)",
			totalSekarang+kuotaBaru, kuotaKategori, kuotaKategori-totalSekarang)
	}
	return nil
}

// ===== Buka penjualan (DRAFT -> OPEN) =====

// ValidasiBukaPenjualan memeriksa semua syarat sebelum event dibuka.
// Semua masalah dikumpulkan sekaligus supaya admin bisa memperbaikinya dalam satu kali kerja.
// Daftar kosong berarti event siap dibuka.
func ValidasiBukaPenjualan(e Event, kategori []Kategori, now time.Time) []string {
	var masalah []string

	if !e.TanggalEvent.After(now) {
		masalah = append(masalah, "Tanggal dan jam event sudah lewat")
	}

	// BR-01: event harus punya kategori tiket
	if len(kategori) == 0 {
		masalah = append(masalah, "Event belum punya kategori tiket")
	}

	// BR-02: total kuota kategori tidak boleh melebihi kapasitas
	totalKategori := 0
	for _, k := range kategori {
		totalKategori += k.Kuota
	}
	if totalKategori > e.Kapasitas {
		masalah = append(masalah, fmt.Sprintf("Total kuota kategori (%d) melebihi kapasitas event (%d)", totalKategori, e.Kapasitas))
	}

	for _, k := range kategori {
		// BR-03: setiap kategori punya tahap harga
		if len(k.Tahap) == 0 {
			masalah = append(masalah, fmt.Sprintf("Kategori %s belum punya tahap harga", k.Nama))
			continue
		}

		// BR-04: total kuota tahap harus sama dengan kuota kategori
		totalTahap := 0
		for _, t := range k.Tahap {
			totalTahap += t.Kuota
		}
		if totalTahap != k.Kuota {
			masalah = append(masalah, fmt.Sprintf("Kategori %s: total kuota tahap %d, harus sama dengan kuota kategori %d", k.Nama, totalTahap, k.Kuota))
		}

		tahap := urutkanTahap(k.Tahap)
		for i, t := range tahap {
			// batas tanggal tahap tidak boleh melewati tanggal event
			if t.BatasTanggal.After(e.TanggalEvent) {
				masalah = append(masalah, fmt.Sprintf("Kategori %s: batas tanggal %s melewati tanggal event", k.Nama, t.Nama))
			}
			// batas tanggal tahap berikutnya tidak boleh lebih awal dari tahap sebelumnya,
			// mis. Presale 2 tidak boleh berakhir sebelum Presale 1
			if i > 0 && t.BatasTanggal.Before(tahap[i-1].BatasTanggal) {
				masalah = append(masalah, fmt.Sprintf("Kategori %s: batas tanggal %s lebih awal dari %s", k.Nama, t.Nama, tahap[i-1].Nama))
			}
		}
	}

	return masalah
}

// ===== Tahap aktif =====

// TahapAktif (BR-05) mengembalikan tahap pertama (berdasarkan urutan) yang
// masih punya sisa kuota DAN batas tanggalnya belum lewat.
// Jika tidak ada, ada = false, artinya kategori tersebut habis.
// Tepat pada detik batas tanggal, tahap masih dianggap aktif.
func TahapAktif(tahap []Tahap, now time.Time) (Tahap, bool) {
	for _, t := range urutkanTahap(tahap) {
		masihAdaKuota := t.SisaKuota > 0
		belumLewat := !now.After(t.BatasTanggal)
		if masihAdaKuota && belumLewat {
			return t, true
		}
	}
	return Tahap{}, false
}

// urutkanTahap mengembalikan salinan tahap yang diurutkan berdasarkan kolom urutan
// (slice asli tidak diubah)
func urutkanTahap(tahap []Tahap) []Tahap {
	salinan := make([]Tahap, len(tahap))
	copy(salinan, tahap)
	sort.Slice(salinan, func(i, j int) bool {
		return salinan[i].Urutan < salinan[j].Urutan
	})
	return salinan
}
