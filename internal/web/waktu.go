package web

import (
	"time"
	_ "time/tzdata" // data zona waktu ikut di dalam program, supaya Asia/Jakarta selalu dikenali
)

// Jakarta adalah zona waktu yang dipakai seluruh aplikasi (WIB, UTC+7)
var Jakarta = muatJakarta()

func muatJakarta() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		panic(err) // tidak akan terjadi karena time/tzdata sudah di-import
	}
	return loc
}

// FormatWaktuForm adalah format nilai <input type="datetime-local">, mis. "2026-12-20T19:00"
const FormatWaktuForm = "2006-01-02T15:04"

// ParseWaktuForm membaca nilai input datetime-local sebagai waktu Asia/Jakarta.
// Browser tidak mengirim zona waktu, jadi zonanya ditentukan di sini.
func ParseWaktuForm(s string) (time.Time, error) {
	return time.ParseInLocation(FormatWaktuForm, s, Jakarta)
}

// WaktuForm kebalikan dari ParseWaktuForm: mengisi value input datetime-local.
// Waktu kosong menghasilkan teks kosong (form tambah baru).
func WaktuForm(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(Jakarta).Format(FormatWaktuForm)
}

var namaBulan = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// Tanggal memformat waktu untuk ditampilkan, mis. "20 Des 2026 19:00 WIB".
// Dipakai di template sebagai {{tanggal .TanggalEvent}}.
func Tanggal(t time.Time) string {
	t = t.In(Jakarta)
	return t.Format("02") + " " + namaBulan[t.Month()-1] + " " + t.Format("2006 15:04") + " WIB"
}
