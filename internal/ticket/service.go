// Package ticket mengurus pembatalan tiket oleh pembeli, pembatalan event oleh admin,
// dan pengembalian dana (BR-17 s.d. BR-20).
package ticket

import (
	"fmt"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/web"
)

// Persentase refund
const (
	PersenRefundPembeli = 75  // BR-18: pembeli membatalkan sendiri
	PersenRefundEvent   = 100 // BR-20: event dibatalkan penyelenggara
)

// HariMinimalBatal adalah batas pembatalan oleh pembeli: paling lambat H-7 (BR-17)
const HariMinimalBatal = 7

// Alasan refund, sama dengan ENUM kolom refunds.alasan
const (
	AlasanPembeli         = "PEMBELI"
	AlasanEventDibatalkan = "EVENT_DIBATALKAN"
)

// KesalahanInput adalah kesalahan karena aturan bisnis (mis. sudah lewat H-7).
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

// tanggalSaja mengambil tanggal kalender di Asia/Jakarta (jam dibuang)
func tanggalSaja(t time.Time) time.Time {
	t = t.In(web.Jakarta)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, web.Jakarta)
}

// BatasBatal (BR-17) adalah tanggal terakhir pembeli boleh membatalkan: tanggal event dikurangi 7 hari.
// Pembatalan masih boleh sepanjang hari itu (sampai 23:59:59 WIB).
func BatasBatal(tanggalEvent time.Time) time.Time {
	return tanggalSaja(tanggalEvent).AddDate(0, 0, -HariMinimalBatal)
}

// BolehBatal (BR-17) bernilai true jika hari ini (tanggal Jakarta) belum melewati BatasBatal.
// Jam tidak dihitung: event 20 Des pukul 19:00 -> boleh dibatalkan sampai 13 Des pukul 23:59:59.
func BolehBatal(tanggalEvent, now time.Time) bool {
	return !tanggalSaja(now).After(BatasBatal(tanggalEvent))
}

// HitungRefund (BR-18, BR-20) menghitung refund dari harga yang dibayar (setelah potongan promo).
// Pembagian bilangan bulat membulatkan ke bawah.
func HitungRefund(hargaBayar int64, persen int) int64 {
	return hargaBayar * int64(persen) / 100
}
