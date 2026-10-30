// Package checkin mengurus pemeriksaan tiket di pintu masuk (BR-24 s.d. BR-26).
package checkin

import (
	"fmt"
	"strings"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/web"
)

// Status tiket yang relevan di pintu (sama dengan ENUM tickets.status)
const (
	tiketHeld      = "HELD"
	tiketActive    = "ACTIVE"
	tiketUsed      = "USED"
	tiketCancelled = "CANCELLED"
	tiketRefunded  = "REFUNDED"
)

// KesalahanInput adalah alasan tiket ditolak di pintu. Pesannya ditampilkan ke petugas.
type KesalahanInput struct {
	Pesan string
}

func (k KesalahanInput) Error() string {
	return k.Pesan
}

func salahInput(format string, a ...any) error {
	return KesalahanInput{Pesan: fmt.Sprintf(format, a...)}
}

// ErrKodeTidakDitemukan: kode tidak ada di tiket mana pun, baik sebagai kode berlaku maupun kode lama
var ErrKodeTidakDitemukan = KesalahanInput{Pesan: "Kode tiket tidak ditemukan. Periksa kembali kode yang diketik."}

// PesanKodeLama adalah penolakan untuk kode lama hasil pemindahan tiket (BR-22)
func PesanKodeLama(namaPenerima string) error {
	return salahInput("Kode lama: tiket ini sudah dipindahkan ke %s. Minta pemegang menunjukkan kode yang baru.", namaPenerima)
}

// InfoTiket adalah data tiket yang ditampilkan dan diperiksa di pintu (BR-24)
type InfoTiket struct {
	ID           int64
	Kode         string
	EventID      int64
	NamaEvent    string
	TanggalEvent time.Time
	NamaPemegang string
	NamaKategori string
	Status       string
	WaktuMasuk   time.Time // terisi jika sudah check-in (USED)
}

// NormalisasiKode membuang semua spasi dan mengubah ke huruf besar,
// supaya "tkt-7k3m 9qx2" yang diketik petugas tetap ditemukan
func NormalisasiKode(kode string) string {
	return strings.ToUpper(strings.Join(strings.Fields(kode), ""))
}

// SamaHari (BR-25) bernilai true jika kedua waktu jatuh pada tanggal kalender yang sama di Asia/Jakarta
func SamaHari(a, b time.Time) bool {
	a, b = a.In(web.Jakarta), b.In(web.Jakarta)
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// CekMasuk (BR-25, BR-26) memeriksa apakah tiket boleh masuk ke event yang sedang dijaga petugas.
// Urutan pemeriksaan: event yang sesuai, status tiket, lalu hari event.
func CekMasuk(t InfoTiket, eventID int64, now time.Time) error {
	if t.EventID != eventID {
		return salahInput("Tiket ini untuk event %s, bukan event yang sedang diperiksa.", t.NamaEvent)
	}

	switch t.Status {
	case tiketActive:
		// boleh, lanjut cek hari event
	case tiketUsed:
		// BR-26: hanya bisa dipakai masuk satu kali
		return salahInput("Tiket sudah dipakai masuk pada %s.", web.Tanggal(t.WaktuMasuk))
	case tiketRefunded:
		return salahInput("Tiket sudah dibatalkan dan dananya dikembalikan.")
	case tiketCancelled:
		return salahInput("Tiket dibatalkan (pesanan tidak jadi).")
	case tiketHeld:
		return salahInput("Tiket belum dibayar.")
	default:
		return salahInput("Tiket tidak bisa dipakai (status %s).", t.Status)
	}

	// BR-25: hanya pada hari event (tanggal kalender WIB)
	if !SamaHari(t.TanggalEvent, now) {
		return salahInput("Bukan hari event. Tiket berlaku tanggal %s.", web.TanggalSaja(t.TanggalEvent))
	}
	return nil
}
