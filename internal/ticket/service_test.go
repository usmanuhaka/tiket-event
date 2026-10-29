package ticket

import (
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/web"
)

func wib(bulan time.Month, tgl, jam, menit, detik int) time.Time {
	return time.Date(2026, bulan, tgl, jam, menit, detik, 0, web.Jakarta)
}

// ===== BR-17 =====

func TestBolehBatalPerTanggalKalender(t *testing.T) {
	event := wib(time.December, 20, 19, 0, 0) // 20 Des 19:00 WIB

	kasus := []struct {
		nama string
		now  time.Time
		mau  bool
	}{
		{"jauh sebelumnya", wib(time.November, 1, 10, 0, 0), true},
		{"13 Des pagi (H-7)", wib(time.December, 13, 0, 0, 0), true},
		{"13 Des 23:59:59 (detik terakhir H-7)", wib(time.December, 13, 23, 59, 59), true},
		{"14 Des 00:00 (H-6)", wib(time.December, 14, 0, 0, 0), false},
		{"hari event", wib(time.December, 20, 8, 0, 0), false},
		// now dalam UTC tetap dihitung per tanggal Jakarta: 13 Des 17:30 UTC = 14 Des 00:30 WIB
		{"UTC 13 Des 17:30 = WIB 14 Des 00:30", time.Date(2026, time.December, 13, 17, 30, 0, 0, time.UTC), false},
		{"UTC 13 Des 16:59 = WIB 13 Des 23:59", time.Date(2026, time.December, 13, 16, 59, 0, 0, time.UTC), true},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if got := BolehBatal(event, k.now); got != k.mau {
				t.Errorf("BolehBatal = %v, mau %v", got, k.mau)
			}
		})
	}
}

func TestJamEventTidakMenggeserHari(t *testing.T) {
	// event dini hari 20 Des 00:30 tetap berbatas 13 Des, bukan 12 Des
	event := wib(time.December, 20, 0, 30, 0)
	if !BolehBatal(event, wib(time.December, 13, 22, 0, 0)) {
		t.Error("13 Des ditolak untuk event 20 Des 00:30")
	}
	if got := BatasBatal(event); !got.Equal(wib(time.December, 13, 0, 0, 0)) {
		t.Errorf("BatasBatal = %v, mau 13 Des", got)
	}
}

// ===== BR-18 dan BR-20 =====

func TestHitungRefund(t *testing.T) {
	kasus := []struct {
		bayar  int64
		persen int
		mau    int64
	}{
		{297500, PersenRefundPembeli, 223125},
		{99999, PersenRefundPembeli, 74999},   // 74.999,25 dibulatkan ke bawah
		{150001, PersenRefundPembeli, 112500}, // 112.500,75 dibulatkan ke bawah
		{297500, PersenRefundEvent, 297500},
		{0, PersenRefundPembeli, 0}, // tiket gratis (promo 100%)
	}
	for _, k := range kasus {
		if got := HitungRefund(k.bayar, k.persen); got != k.mau {
			t.Errorf("HitungRefund(%d, %d%%) = %d, mau %d", k.bayar, k.persen, got, k.mau)
		}
	}
}
