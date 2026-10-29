package event

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// waktu patokan semua test: 1 Desember 2026 10:00 WIB
var jakarta = time.FixedZone("WIB", 7*3600)
var sekarang = time.Date(2026, 12, 1, 10, 0, 0, 0, jakarta)

func hari(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// ===== BR-05: tahap aktif =====

func tigaTahap() []Tahap {
	return []Tahap{
		{ID: 1, Nama: "Presale 1", Urutan: 1, Harga: 100000, Kuota: 10, SisaKuota: 10, BatasTanggal: sekarang.Add(hari(5))},
		{ID: 2, Nama: "Presale 2", Urutan: 2, Harga: 125000, Kuota: 10, SisaKuota: 10, BatasTanggal: sekarang.Add(hari(10))},
		{ID: 3, Nama: "Normal", Urutan: 3, Harga: 150000, Kuota: 10, SisaKuota: 10, BatasTanggal: sekarang.Add(hari(20))},
	}
}

func TestTahapAktif(t *testing.T) {
	kasus := []struct {
		nama  string
		ubah  func(tahap []Tahap) []Tahap
		now   time.Time
		mauID int64 // 0 berarti kategori habis
	}{
		{"tahap 1 masih ada kuota dan tanggal", func(t []Tahap) []Tahap { return t }, sekarang, 1},
		{"tahap 1 kuota habis -> tahap 2", func(t []Tahap) []Tahap { t[0].SisaKuota = 0; return t }, sekarang, 2},
		{"tahap 1 masih ada kuota tapi tanggal lewat -> tahap 2", func(t []Tahap) []Tahap { return t }, sekarang.Add(hari(5) + time.Second), 2},
		{"tepat di batas tanggal tahap 1 masih aktif", func(t []Tahap) []Tahap { return t }, sekarang.Add(hari(5)), 1},
		{"tahap 1 dan 2 habis -> tahap 3", func(t []Tahap) []Tahap { t[0].SisaKuota, t[1].SisaKuota = 0, 0; return t }, sekarang, 3},
		{"semua kuota habis -> kategori habis", func(t []Tahap) []Tahap {
			for i := range t {
				t[i].SisaKuota = 0
			}
			return t
		}, sekarang, 0},
		{"semua tanggal lewat -> kategori habis", func(t []Tahap) []Tahap { return t }, sekarang.Add(hari(21)), 0},
		{"input tidak urut tetap memilih urutan terkecil", func(t []Tahap) []Tahap { return []Tahap{t[2], t[0], t[1]} }, sekarang, 1},
		{"tanpa tahap -> kategori habis", func(t []Tahap) []Tahap { return nil }, sekarang, 0},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			aktif, ada := TahapAktif(k.ubah(tigaTahap()), k.now)
			if k.mauID == 0 {
				if ada {
					t.Errorf("mau habis, dapat tahap %s", aktif.Nama)
				}
				return
			}
			if !ada || aktif.ID != k.mauID {
				t.Errorf("mau tahap id %d, dapat id %d (ada=%v)", k.mauID, aktif.ID, ada)
			}
		})
	}
}

func TestTahapTerlihatMenyembunyikanKuotaNol(t *testing.T) {
	tahap := tigaTahap()
	tahap[0].Kuota, tahap[0].SisaKuota = 0, 0

	terlihat := TahapTerlihat(tahap)
	if len(terlihat) != 2 || terlihat[0].ID != 2 || terlihat[1].ID != 3 {
		t.Errorf("dapat %+v, mau tahap id 2 dan 3", terlihat)
	}
}

// ===== BR-02: kuota kategori vs kapasitas =====

func TestCekKuotaKategori(t *testing.T) {
	if err := CekKuotaKategori(500, 300, 100); err != nil {
		t.Errorf("di bawah kapasitas ditolak: %v", err)
	}
	if err := CekKuotaKategori(500, 400, 100); err != nil {
		t.Errorf("tepat sama dengan kapasitas ditolak: %v", err)
	}
	err := CekKuotaKategori(500, 400, 101)
	var salah KesalahanInput
	if !errors.As(err, &salah) {
		t.Fatalf("melebihi kapasitas harus ditolak sebagai KesalahanInput, dapat %v", err)
	}
	if !strings.Contains(salah.Pesan, "sisa yang bisa dibagi: 100") {
		t.Errorf("pesan kurang jelas: %q", salah.Pesan)
	}
}

func TestCekKapasitasBaru(t *testing.T) {
	if err := CekKapasitasBaru(500, 500); err != nil {
		t.Errorf("kapasitas sama dengan total kategori ditolak: %v", err)
	}
	if err := CekKapasitasBaru(499, 500); err == nil {
		t.Error("kapasitas di bawah total kuota kategori diterima")
	}
}

// ===== BR-04: kuota tahap vs kuota kategori (saat menambah tahap) =====

func TestCekKuotaTahap(t *testing.T) {
	if err := CekKuotaTahap(400, 250, 150); err != nil {
		t.Errorf("tepat sama dengan kuota kategori ditolak: %v", err)
	}
	if err := CekKuotaTahap(400, 250, 100); err != nil {
		t.Errorf("di bawah kuota kategori ditolak: %v", err)
	}
	if err := CekKuotaTahap(400, 250, 151); err == nil {
		t.Error("melebihi kuota kategori diterima")
	}
}

// ===== Validasi form =====

func TestValidasiEvent(t *testing.T) {
	valid := Event{Nama: "Konser", Lokasi: "Aula", Kapasitas: 100, TanggalEvent: sekarang.Add(hari(30))}

	kasus := []struct {
		nama     string
		ubah     func(e *Event)
		mauLulus bool
	}{
		{"valid", func(e *Event) {}, true},
		{"hari ini, jamnya belum lewat", func(e *Event) { e.TanggalEvent = sekarang.Add(time.Hour) }, true},
		{"hari ini, jamnya sudah lewat", func(e *Event) { e.TanggalEvent = sekarang.Add(-time.Hour) }, false},
		{"tepat sekarang dianggap lewat", func(e *Event) { e.TanggalEvent = sekarang }, false},
		{"nama kosong", func(e *Event) { e.Nama = "  " }, false},
		{"lokasi kosong", func(e *Event) { e.Lokasi = "" }, false},
		{"kapasitas 0", func(e *Event) { e.Kapasitas = 0 }, false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			e := valid
			k.ubah(&e)
			err := ValidasiEvent(e, sekarang)
			if k.mauLulus != (err == nil) {
				t.Errorf("mau lulus=%v, dapat error %v", k.mauLulus, err)
			}
		})
	}
}

func TestValidasiKategoriDanTahap(t *testing.T) {
	if ValidasiKategori("VIP", 100) != nil {
		t.Error("kategori valid ditolak")
	}
	if ValidasiKategori("", 100) == nil || ValidasiKategori("VIP", 0) == nil {
		t.Error("kategori tanpa nama atau kuota 0 diterima")
	}

	tahap := Tahap{Nama: "Presale 1", Harga: 100000, Kuota: 10, BatasTanggal: sekarang}
	if ValidasiTahap(tahap) != nil {
		t.Error("tahap valid ditolak")
	}
	gratis := tahap
	gratis.Harga = 0
	if ValidasiTahap(gratis) != nil {
		t.Error("tahap gratis (harga 0) ditolak")
	}
	minus := tahap
	minus.Harga = -1
	tanpaBatas := tahap
	tanpaBatas.BatasTanggal = time.Time{}
	if ValidasiTahap(minus) == nil || ValidasiTahap(tanpaBatas) == nil {
		t.Error("harga minus atau batas tanggal kosong diterima")
	}
}

// ===== Buka penjualan (BR-01 s.d. BR-04 sekaligus) =====

// eventSiapDibuka: kapasitas 30, satu kategori 30 dengan 3 tahap 10+10+10
func eventSiapDibuka() (Event, []Kategori) {
	e := Event{Nama: "Konser", Kapasitas: 30, TanggalEvent: sekarang.Add(hari(30)), Status: StatusDraft}
	k := []Kategori{{Nama: "Festival", Kuota: 30, Tahap: tigaTahap()}}
	return e, k
}

func TestValidasiBukaPenjualan(t *testing.T) {
	kasus := []struct {
		nama       string
		ubah       func(e *Event, k []Kategori) []Kategori
		mauMasalah string // kosong berarti harus lolos
	}{
		{"lengkap dan benar", func(e *Event, k []Kategori) []Kategori { return k }, ""},
		{"tanggal event lewat", func(e *Event, k []Kategori) []Kategori {
			e.TanggalEvent = sekarang.Add(-time.Hour)
			return k
		}, "sudah lewat"},
		{"BR-01 tanpa kategori", func(e *Event, k []Kategori) []Kategori { return nil }, "belum punya kategori"},
		{"BR-02 total kategori melebihi kapasitas", func(e *Event, k []Kategori) []Kategori {
			e.Kapasitas = 20
			return k
		}, "melebihi kapasitas"},
		{"BR-03 kategori tanpa tahap", func(e *Event, k []Kategori) []Kategori {
			k[0].Tahap = nil
			return k
		}, "belum punya tahap"},
		{"BR-04 total tahap kurang dari kuota kategori", func(e *Event, k []Kategori) []Kategori {
			k[0].Tahap = k[0].Tahap[:2]
			return k
		}, "harus sama dengan kuota kategori"},
		{"batas tahap melewati tanggal event", func(e *Event, k []Kategori) []Kategori {
			k[0].Tahap[2].BatasTanggal = e.TanggalEvent.Add(time.Hour)
			return k
		}, "melewati tanggal event"},
		{"batas tahap 2 lebih awal dari tahap 1", func(e *Event, k []Kategori) []Kategori {
			k[0].Tahap[1].BatasTanggal = k[0].Tahap[0].BatasTanggal.Add(-time.Hour)
			return k
		}, "Presale 2 lebih awal dari Presale 1"},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			e, kategori := eventSiapDibuka()
			kategori = k.ubah(&e, kategori)
			masalah := ValidasiBukaPenjualan(e, kategori, sekarang)

			if k.mauMasalah == "" {
				if len(masalah) != 0 {
					t.Errorf("mau lolos, dapat masalah %v", masalah)
				}
				return
			}
			gabung := strings.Join(masalah, " | ")
			if !strings.Contains(gabung, k.mauMasalah) {
				t.Errorf("mau masalah berisi %q, dapat %v", k.mauMasalah, masalah)
			}
		})
	}
}

func TestValidasiBukaPenjualanBatasTanggalSamaBoleh(t *testing.T) {
	// tahap 2 berakhir di waktu yang sama dengan tahap 1: tidak lebih awal, jadi boleh
	e, kategori := eventSiapDibuka()
	kategori[0].Tahap[1].BatasTanggal = kategori[0].Tahap[0].BatasTanggal
	if masalah := ValidasiBukaPenjualan(e, kategori, sekarang); len(masalah) != 0 {
		t.Errorf("batas tanggal sama ditolak: %v", masalah)
	}
}

func TestValidasiBukaPenjualanMengumpulkanSemuaMasalah(t *testing.T) {
	e, kategori := eventSiapDibuka()
	e.Kapasitas = 20                          // BR-02 gagal
	kategori[0].Tahap = kategori[0].Tahap[:1] // BR-04 gagal

	if masalah := ValidasiBukaPenjualan(e, kategori, sekarang); len(masalah) != 2 {
		t.Errorf("mau 2 masalah sekaligus, dapat %d: %v", len(masalah), masalah)
	}
}
