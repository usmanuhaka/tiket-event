// Package order mengurus pemesanan tiket, penahanan 15 menit, pembayaran simulasi,
// dan kedaluwarsa pesanan (BR-06 s.d. BR-12).
package order

import (
	"crypto/rand"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/usmanuhaka/tiket-event/internal/event"
)

// Status pesanan, sama dengan ENUM kolom orders.status
const (
	StatusHeld      = "HELD"
	StatusPaid      = "PAID"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"
)

// Status tiket, sama dengan ENUM kolom tickets.status
const (
	TiketHeld      = "HELD"
	TiketActive    = "ACTIVE"
	TiketCancelled = "CANCELLED"
)

// MaksTiket adalah jumlah tiket terbanyak dalam satu transaksi (BR-06)
const MaksTiket = 4

// LamaTahan adalah lama tiket ditahan sebelum harus dibayar (BR-09)
const LamaTahan = 15 * time.Minute

// KesalahanInput adalah kesalahan karena isi form atau aturan bisnis.
// Handler menampilkan pesannya ke pembeli dengan status 400.
type KesalahanInput struct {
	Pesan string
}

func (k KesalahanInput) Error() string {
	return k.Pesan
}

func salahInput(format string, a ...any) error {
	return KesalahanInput{Pesan: fmt.Sprintf(format, a...)}
}

// BarisTiket adalah satu baris di form pesan: kategori yang dipilih dan nama pemegangnya
type BarisTiket struct {
	KategoriID   int64
	NamaPemegang string
}

// Alokasi adalah hasil penentuan harga untuk satu tiket: dari tahap mana dan berapa harganya
type Alokasi struct {
	TahapID   int64
	NamaTahap string
	Harga     int64
}

// ValidasiPesanan memeriksa jumlah tiket (BR-06) dan nama pemegang (BR-07)
func ValidasiPesanan(baris []BarisTiket) error {
	// BR-06: satu transaksi berisi 1-4 tiket
	if len(baris) == 0 {
		return salahInput("Isi minimal 1 tiket")
	}
	if len(baris) > MaksTiket {
		return salahInput("Satu pesanan maksimal %d tiket", MaksTiket)
	}

	// BR-07: setiap tiket wajib mencantumkan nama pemegang
	for i, b := range baris {
		nama := strings.TrimSpace(b.NamaPemegang)
		if nama == "" {
			return salahInput("Nama pemegang tiket ke-%d wajib diisi", i+1)
		}
		if utf8.RuneCountInString(nama) > 100 {
			return salahInput("Nama pemegang tiket ke-%d maksimal 100 karakter", i+1)
		}
	}
	return nil
}

// AlokasiTiket (BR-08) menentukan harga setiap tiket dari satu kategori dengan alokasi berurutan:
// tahap disusun menurut urutan, tahap yang batas tanggalnya sudah lewat dilewati,
// lalu tiket diambil dari sisa kuota tahap pertama, kekurangannya dari tahap berikutnya, dst.
// Contoh: tahap aktif tinggal 1 kuota dan pembeli memesan 3 -> 1 tiket harga tahap itu,
// 2 tiket harga tahap berikutnya.
// Jika total sisa tidak cukup, seluruh permintaan ditolak (tidak ada pesanan sebagian).
func AlokasiTiket(tahap []event.Tahap, jumlah int, now time.Time) ([]Alokasi, error) {
	urut := make([]event.Tahap, len(tahap))
	copy(urut, tahap)
	sort.Slice(urut, func(i, j int) bool { return urut[i].Urutan < urut[j].Urutan })

	var hasil []Alokasi
	kurang := jumlah
	for _, t := range urut {
		if kurang == 0 {
			break
		}
		if now.After(t.BatasTanggal) {
			continue // tahap sudah lewat, tidak bisa dijual lagi (BR-05)
		}
		ambil := min(t.SisaKuota, kurang)
		for i := 0; i < ambil; i++ {
			hasil = append(hasil, Alokasi{TahapID: t.ID, NamaTahap: t.Nama, Harga: t.Harga})
		}
		kurang -= ambil
	}

	if kurang > 0 {
		return nil, salahInput("Kuota tidak cukup: tersisa %d tiket, diminta %d", jumlah-kurang, jumlah)
	}
	return hasil, nil
}

// BatasBayar (BR-09) adalah waktu terakhir pesanan boleh dibayar
func BatasBayar(waktuPesan time.Time) time.Time {
	return waktuPesan.Add(LamaTahan)
}

// SudahKedaluwarsa (BR-10, BR-11) bernilai true jika waktu bayar sudah lewat.
// Tepat pada detik batas, pesanan masih boleh dibayar.
func SudahKedaluwarsa(batasBayar, now time.Time) bool {
	return now.After(batasBayar)
}

// hurufKode berisi 32 huruf dan angka tanpa 0/O dan 1/I, supaya kode tidak salah baca di pintu masuk
const hurufKode = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// BuatKodeTiket (BR-11) membuat kode tiket acak, mis. "TKT-7K3M9QX2".
// 8 karakter dari 32 pilihan = sekitar 1 triliun kemungkinan.
// Keunikan tetap dijaga kolom UNIQUE di database (lihat bayarPesanan).
func BuatKodeTiket() string {
	b := make([]byte, 8)
	rand.Read(b) // crypto/rand: tidak bisa ditebak; sejak Go 1.24 tidak pernah error
	kode := make([]byte, 8)
	for i := range b {
		// 256 habis dibagi 32, jadi setiap huruf punya peluang yang sama
		kode[i] = hurufKode[int(b[i])%len(hurufKode)]
	}
	return "TKT-" + string(kode)
}
