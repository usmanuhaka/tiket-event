// Package promo mengurus kode promo (BR-13 s.d. BR-16).
package promo

import (
	"fmt"
	"strings"
)

// Promo adalah satu baris tabel promo_codes beserta daftar kategorinya
type Promo struct {
	ID             int64
	Kode           string
	Persen         int
	KuotaPemakaian int
	Terpakai       int
	KategoriID     []int64  // kategori tempat promo berlaku (BR-14)
	NamaKategori   []string // untuk tampilan admin, mis. "Konser Akhir Tahun - VIP"
}

// KesalahanInput adalah kesalahan karena isi form atau aturan bisnis.
// Handler menampilkan pesannya dengan status 400.
type KesalahanInput struct {
	Pesan string
}

func (k KesalahanInput) Error() string {
	return k.Pesan
}

func salahInput(format string, a ...any) error {
	return KesalahanInput{Pesan: fmt.Sprintf(format, a...)}
}

// NormalisasiKode membuang spasi dan mengubah ke huruf besar,
// supaya "hemat15 " dan "HEMAT15" dianggap kode yang sama
func NormalisasiKode(kode string) string {
	return strings.ToUpper(strings.TrimSpace(kode))
}

// ValidasiPromo (BR-13) memeriksa isi form promo. Kode harus sudah dinormalisasi.
func ValidasiPromo(p Promo) error {
	if len(p.Kode) < 3 || len(p.Kode) > 30 {
		return salahInput("Kode promo harus 3 sampai 30 karakter")
	}
	for _, c := range p.Kode {
		hurufBesar := c >= 'A' && c <= 'Z'
		angka := c >= '0' && c <= '9'
		if !hurufBesar && !angka && c != '-' {
			return salahInput("Kode promo hanya boleh berisi huruf, angka, dan tanda -")
		}
	}
	if p.Persen < 1 || p.Persen > 100 {
		return salahInput("Persentase potongan harus antara 1 dan 100")
	}
	if p.KuotaPemakaian < 1 {
		return salahInput("Kuota pemakaian minimal 1")
	}
	if len(p.KategoriID) == 0 {
		return salahInput("Pilih minimal 1 kategori tempat promo berlaku")
	}
	return nil
}

// HitungPotongan (BR-14) menghitung potongan satu tiket: harga x persen / 100.
// Pembagian bilangan bulat di Go membulatkan ke bawah, jadi pembeli tidak pernah
// mendapat potongan lebih besar dari persentasenya (mis. 10% dari Rp99.999 = Rp9.999).
func HitungPotongan(harga int64, persen int) int64 {
	return harga * int64(persen) / 100
}

// BerlakuUntuk (BR-14) bernilai true jika promo berlaku untuk kategori tiket ini
func (p Promo) BerlakuUntuk(kategoriID int64) bool {
	for _, id := range p.KategoriID {
		if id == kategoriID {
			return true
		}
	}
	return false
}

// MasihAdaKuota (BR-16) bernilai false jika kuota pemakaian promo sudah habis
func (p Promo) MasihAdaKuota() bool {
	return p.Terpakai < p.KuotaPemakaian
}
