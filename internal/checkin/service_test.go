package checkin

import (
	"strings"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/web"
)

func wib(tgl, jam, menit, detik int) time.Time {
	return time.Date(2026, time.December, tgl, jam, menit, detik, 0, web.Jakarta)
}

func TestNormalisasiKode(t *testing.T) {
	if got := NormalisasiKode("  tkt-7k3m 9qx2 "); got != "TKT-7K3M9QX2" {
		t.Errorf("dapat %q", got)
	}
}

func TestSamaHari(t *testing.T) {
	event := wib(20, 19, 0, 0)
	kasus := []struct {
		nama string
		now  time.Time
		mau  bool
	}{
		{"00:00 hari event", wib(20, 0, 0, 0), true},
		{"23:59:59 hari event", wib(20, 23, 59, 59), true},
		{"23:59:59 sehari sebelumnya", wib(19, 23, 59, 59), false},
		{"00:00 sehari sesudahnya", wib(21, 0, 0, 0), false},
		// 19 Des 17:30 UTC = 20 Des 00:30 WIB
		{"UTC yang sudah masuk tanggal 20 di WIB", time.Date(2026, time.December, 19, 17, 30, 0, 0, time.UTC), true},
	}
	for _, k := range kasus {
		if got := SamaHari(event, k.now); got != k.mau {
			t.Errorf("%s: dapat %v, mau %v", k.nama, got, k.mau)
		}
	}
}

func TestCekMasuk(t *testing.T) {
	valid := InfoTiket{EventID: 1, NamaEvent: "Konser Akhir Tahun", TanggalEvent: wib(20, 19, 0, 0), Status: "ACTIVE"}
	hariEvent := wib(20, 17, 0, 0)

	kasus := []struct {
		nama  string
		ubah  func(t *InfoTiket)
		event int64
		now   time.Time
		pesan string // kosong berarti boleh masuk
	}{
		{"ACTIVE di hari event", func(t *InfoTiket) {}, 1, hariEvent, ""},
		{"event lain", func(t *InfoTiket) {}, 2, hariEvent, "untuk event Konser Akhir Tahun"},
		{"REFUNDED", func(t *InfoTiket) { t.Status = "REFUNDED" }, 1, hariEvent, "dananya dikembalikan"},
		{"CANCELLED", func(t *InfoTiket) { t.Status = "CANCELLED" }, 1, hariEvent, "dibatalkan"},
		{"HELD", func(t *InfoTiket) { t.Status = "HELD" }, 1, hariEvent, "belum dibayar"},
		{"USED beserta waktunya", func(t *InfoTiket) {
			t.Status = "USED"
			t.WaktuMasuk = wib(20, 18, 42, 0)
		}, 1, hariEvent, "sudah dipakai masuk pada 20 Des 2026 18:42 WIB"},
		{"sehari sebelum event", func(t *InfoTiket) {}, 1, wib(19, 20, 0, 0), "Bukan hari event. Tiket berlaku tanggal 20 Des 2026"},
		{"sehari sesudah event", func(t *InfoTiket) {}, 1, wib(21, 1, 0, 0), "Bukan hari event"},
		{"event lain dicek sebelum status", func(t *InfoTiket) { t.Status = "USED" }, 2, hariEvent, "untuk event"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			info := valid
			k.ubah(&info)
			err := CekMasuk(info, k.event, k.now)
			if k.pesan == "" {
				if err != nil {
					t.Errorf("mau boleh masuk, dapat %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), k.pesan) {
				t.Errorf("mau ditolak berisi %q, dapat %v", k.pesan, err)
			}
		})
	}
}

func TestPesanKodeLamaMenyebutPenerima(t *testing.T) {
	if err := PesanKodeLama("Rina Penerima"); !strings.Contains(err.Error(), "sudah dipindahkan ke Rina Penerima") {
		t.Errorf("pesan: %v", err)
	}
}
