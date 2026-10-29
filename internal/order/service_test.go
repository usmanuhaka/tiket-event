package order

import (
	"strings"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/event"
)

var jakarta = time.FixedZone("WIB", 7*3600)
var sekarang = time.Date(2026, 12, 1, 10, 0, 0, 0, jakarta)

func hari(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// ===== BR-06 dan BR-07 =====

func TestValidasiPesanan(t *testing.T) {
	satu := BarisTiket{KategoriID: 1, NamaPemegang: "Budi"}
	kasus := []struct {
		nama     string
		baris    []BarisTiket
		mauLulus bool
	}{
		{"BR-06 tanpa tiket", nil, false},
		{"BR-06 satu tiket", []BarisTiket{satu}, true},
		{"BR-06 empat tiket", []BarisTiket{satu, satu, satu, satu}, true},
		{"BR-06 lima tiket", []BarisTiket{satu, satu, satu, satu, satu}, false},
		{"BR-07 nama kosong", []BarisTiket{satu, {KategoriID: 1, NamaPemegang: ""}}, false},
		{"BR-07 nama hanya spasi", []BarisTiket{{KategoriID: 1, NamaPemegang: "   "}}, false},
		{"BR-07 nama terlalu panjang", []BarisTiket{{KategoriID: 1, NamaPemegang: strings.Repeat("a", 101)}}, false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			err := ValidasiPesanan(k.baris)
			if k.mauLulus != (err == nil) {
				t.Errorf("mau lulus=%v, dapat %v", k.mauLulus, err)
			}
		})
	}
}

// ===== BR-08: alokasi harga berurutan =====

// tigaTahap: Presale 1 Rp100rb, Presale 2 Rp125rb, Normal Rp150rb
func tigaTahap(sisa1, sisa2, sisa3 int) []event.Tahap {
	return []event.Tahap{
		{ID: 1, Nama: "Presale 1", Urutan: 1, Harga: 100000, Kuota: 10, SisaKuota: sisa1, BatasTanggal: sekarang.Add(hari(5))},
		{ID: 2, Nama: "Presale 2", Urutan: 2, Harga: 125000, Kuota: 10, SisaKuota: sisa2, BatasTanggal: sekarang.Add(hari(10))},
		{ID: 3, Nama: "Normal", Urutan: 3, Harga: 150000, Kuota: 10, SisaKuota: sisa3, BatasTanggal: sekarang.Add(hari(20))},
	}
}

// idTahap meringkas hasil alokasi menjadi daftar id tahap, mis. [1 2 2]
func idTahap(a []Alokasi) []int64 {
	var hasil []int64
	for _, x := range a {
		hasil = append(hasil, x.TahapID)
	}
	return hasil
}

func samaDengan(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAlokasiTiket(t *testing.T) {
	kasus := []struct {
		nama   string
		tahap  []event.Tahap
		jumlah int
		now    time.Time
		mau    []int64 // nil berarti ditolak
	}{
		{"semua dari tahap 1", tigaTahap(10, 10, 10), 3, sekarang, []int64{1, 1, 1}},
		{"contoh PRD: tahap aktif sisa 1, pesan 3", tigaTahap(1, 10, 10), 3, sekarang, []int64{1, 2, 2}},
		{"melintasi tiga tahap", tigaTahap(1, 1, 10), 4, sekarang, []int64{1, 2, 3, 3}},
		{"tahap 1 habis, mulai dari tahap 2", tigaTahap(0, 10, 10), 2, sekarang, []int64{2, 2}},
		{"tahap 1 masih ada sisa tapi tanggal lewat", tigaTahap(10, 10, 10), 2, sekarang.Add(hari(6)), []int64{2, 2}},
		{"kuota pas habis", tigaTahap(1, 1, 2), 4, sekarang, []int64{1, 2, 3, 3}},
		{"kuota kurang -> ditolak", tigaTahap(1, 1, 1), 4, sekarang, nil},
		{"semua tanggal lewat -> ditolak", tigaTahap(10, 10, 10), 1, sekarang.Add(hari(21)), nil},
		{"input tidak urut tetap mulai dari urutan terkecil", func() []event.Tahap {
			t := tigaTahap(1, 10, 10)
			return []event.Tahap{t[2], t[1], t[0]}
		}(), 2, sekarang, []int64{1, 2}},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			hasil, err := AlokasiTiket(k.tahap, k.jumlah, k.now)
			if k.mau == nil {
				if err == nil {
					t.Errorf("mau ditolak, dapat %v", idTahap(hasil))
				}
				return
			}
			if err != nil {
				t.Fatalf("ditolak: %v", err)
			}
			if !samaDengan(idTahap(hasil), k.mau) {
				t.Errorf("dapat tahap %v, mau %v", idTahap(hasil), k.mau)
			}
		})
	}
}

func TestAlokasiTiketHargaPerTiket(t *testing.T) {
	hasil, err := AlokasiTiket(tigaTahap(1, 10, 10), 3, sekarang)
	if err != nil {
		t.Fatal(err)
	}
	mau := []int64{100000, 125000, 125000}
	for i, a := range hasil {
		if a.Harga != mau[i] {
			t.Errorf("tiket ke-%d: harga %d, mau %d", i+1, a.Harga, mau[i])
		}
	}
}

// ===== BR-09, BR-10, BR-11: batas waktu =====

func TestBatasBayarDanKedaluwarsa(t *testing.T) {
	batas := BatasBayar(sekarang)
	if !batas.Equal(sekarang.Add(15 * time.Minute)) {
		t.Errorf("batas bayar %v, mau 15 menit setelah pesan", batas)
	}
	if SudahKedaluwarsa(batas, sekarang.Add(14*time.Minute)) {
		t.Error("menit ke-14 dianggap kedaluwarsa")
	}
	if SudahKedaluwarsa(batas, batas) {
		t.Error("tepat di detik batas seharusnya masih boleh bayar")
	}
	if !SudahKedaluwarsa(batas, batas.Add(time.Second)) {
		t.Error("1 detik setelah batas seharusnya kedaluwarsa")
	}
}

// ===== BR-11: kode tiket =====

func TestBuatKodeTiket(t *testing.T) {
	sudah := map[string]bool{}
	for i := 0; i < 1000; i++ {
		kode := BuatKodeTiket()
		if len(kode) != 12 || !strings.HasPrefix(kode, "TKT-") {
			t.Fatalf("format kode salah: %q", kode)
		}
		for _, c := range kode[4:] {
			if !strings.ContainsRune(hurufKode, c) {
				t.Fatalf("karakter %q tidak diizinkan (kode %s)", c, kode)
			}
		}
		if sudah[kode] {
			t.Fatalf("kode %s muncul dua kali", kode)
		}
		sudah[kode] = true
	}
}
