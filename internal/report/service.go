// Package report menyusun laporan penjualan dan pendapatan per event (BR-27, BR-28).
package report

import "time"

// Angka adalah satu kelompok angka laporan. Dipakai di tiga level: tahap, kategori, dan event.
type Angka struct {
	Terjual     int   // tiket ACTIVE + USED + REFUNDED (pernah dibayar)
	TiketRefund int   // berapa dari yang terjual sudah di-refund
	SudahMasuk  int   // berapa dari yang terjual sudah check-in (USED)
	Kotor       int64 // jumlah harga tahap sebelum promo
	Potongan    int64 // jumlah potongan promo
	Refund      int64 // jumlah uang yang dikembalikan
}

// Bersih (BR-28) = pendapatan kotor - total potongan promo - total refund
func (a Angka) Bersih() int64 {
	return a.Kotor - a.Potongan - a.Refund
}

// Tambah menjumlahkan angka b ke a (dipakai untuk subtotal kategori dan total event)
func (a *Angka) Tambah(b Angka) {
	a.Terjual += b.Terjual
	a.TiketRefund += b.TiketRefund
	a.SudahMasuk += b.SudahMasuk
	a.Kotor += b.Kotor
	a.Potongan += b.Potongan
	a.Refund += b.Refund
}

// BarisTahap adalah satu baris hasil query laporan: angka untuk satu tahap harga
type BarisTahap struct {
	KategoriID   int64
	NamaKategori string
	TahapID      int64
	NamaTahap    string
	Urutan       int
	Harga        int64
	Kuota        int
	SisaKuota    int
	Angka
}

// LaporanKategori adalah satu kategori beserta tahap-tahapnya dan subtotalnya
type LaporanKategori struct {
	ID       int64
	Nama     string
	Tahap    []BarisTahap
	Subtotal Angka
}

// Laporan adalah laporan lengkap satu event (BR-27)
type Laporan struct {
	EventID      int64
	NamaEvent    string
	TanggalEvent time.Time
	StatusEvent  string
	Kategori     []LaporanKategori
	Total        Angka
}

// SusunLaporan mengelompokkan baris tahap (yang sudah urut per kategori lalu per urutan tahap)
// menjadi kategori, lalu menjumlahkan subtotal kategori dan total event.
// Ketiga level dihitung dari baris yang sama, jadi jumlah per tahap = jumlah per kategori = total event.
func SusunLaporan(baris []BarisTahap) ([]LaporanKategori, Angka) {
	var kategori []LaporanKategori
	var total Angka

	for _, b := range baris {
		// kategori baru dimulai ketika id kategori berganti
		if len(kategori) == 0 || kategori[len(kategori)-1].ID != b.KategoriID {
			kategori = append(kategori, LaporanKategori{ID: b.KategoriID, Nama: b.NamaKategori})
		}
		k := &kategori[len(kategori)-1]
		k.Tahap = append(k.Tahap, b)
		k.Subtotal.Tambah(b.Angka)
		total.Tambah(b.Angka)
	}
	return kategori, total
}
