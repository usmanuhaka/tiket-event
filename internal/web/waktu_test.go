package web

import (
	"testing"
	"time"
)

func TestParseWaktuForm(t *testing.T) {
	got, err := ParseWaktuForm("2026-12-20T19:00")
	if err != nil {
		t.Fatal(err)
	}
	// 19:00 WIB sama dengan 12:00 UTC
	mau := time.Date(2026, 12, 20, 12, 0, 0, 0, time.UTC)
	if !got.Equal(mau) {
		t.Errorf("dapat %v, mau %v", got, mau)
	}
	if got.Location().String() != "Asia/Jakarta" {
		t.Errorf("zona %s, mau Asia/Jakarta", got.Location())
	}

	for _, salah := range []string{"", "20-12-2026 19:00", "2026-13-01T10:00", "besok"} {
		if _, err := ParseWaktuForm(salah); err == nil {
			t.Errorf("input %q seharusnya ditolak", salah)
		}
	}
}

func TestWaktuForm(t *testing.T) {
	// waktu UTC tetap ditampilkan dalam jam Jakarta
	utc := time.Date(2026, 12, 20, 12, 0, 0, 0, time.UTC)
	if got := WaktuForm(utc); got != "2026-12-20T19:00" {
		t.Errorf("dapat %q", got)
	}
	if got := WaktuForm(time.Time{}); got != "" {
		t.Errorf("waktu kosong: dapat %q, mau kosong", got)
	}
}

func TestTanggal(t *testing.T) {
	waktu := time.Date(2026, 12, 20, 12, 0, 0, 0, time.UTC)
	if got := Tanggal(waktu); got != "20 Des 2026 19:00 WIB" {
		t.Errorf("dapat %q", got)
	}
	if got := Tanggal(time.Date(2026, 5, 3, 1, 5, 0, 0, Jakarta)); got != "03 Mei 2026 01:05 WIB" {
		t.Errorf("dapat %q", got)
	}
	// 13 Des 18:00 UTC sudah 14 Des di Jakarta
	if got := TanggalSaja(time.Date(2026, 12, 13, 18, 0, 0, 0, time.UTC)); got != "14 Des 2026" {
		t.Errorf("TanggalSaja: dapat %q", got)
	}
}
