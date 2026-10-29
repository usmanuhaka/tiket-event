package promo

import (
	"strings"
	"testing"
)

func TestNormalisasiKode(t *testing.T) {
	if got := NormalisasiKode("  hemat15 "); got != "HEMAT15" {
		t.Errorf("dapat %q, mau HEMAT15", got)
	}
}

// ===== BR-13 =====

func TestValidasiPromo(t *testing.T) {
	valid := Promo{Kode: "HEMAT-15", Persen: 15, KuotaPemakaian: 10, KategoriID: []int64{1}}

	kasus := []struct {
		nama     string
		ubah     func(p *Promo)
		mauLulus bool
	}{
		{"valid", func(p *Promo) {}, true},
		{"persen 1 (batas bawah)", func(p *Promo) { p.Persen = 1 }, true},
		{"persen 100 (batas atas)", func(p *Promo) { p.Persen = 100 }, true},
		{"persen 0", func(p *Promo) { p.Persen = 0 }, false},
		{"persen 101", func(p *Promo) { p.Persen = 101 }, false},
		{"kuota 0", func(p *Promo) { p.KuotaPemakaian = 0 }, false},
		{"tanpa kategori", func(p *Promo) { p.KategoriID = nil }, false},
		{"kode 2 karakter", func(p *Promo) { p.Kode = "AB" }, false},
		{"kode 31 karakter", func(p *Promo) { p.Kode = strings.Repeat("A", 31) }, false},
		{"kode dengan spasi", func(p *Promo) { p.Kode = "HEMAT 15" }, false},
		{"kode huruf kecil (belum dinormalisasi)", func(p *Promo) { p.Kode = "hemat15" }, false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			p := valid
			k.ubah(&p)
			err := ValidasiPromo(p)
			if k.mauLulus != (err == nil) {
				t.Errorf("mau lulus=%v, dapat %v", k.mauLulus, err)
			}
		})
	}
}

// ===== BR-14 =====

func TestHitungPotonganDibulatkanKeBawah(t *testing.T) {
	kasus := []struct {
		harga  int64
		persen int
		mau    int64
	}{
		{150000, 15, 22500},
		{99999, 10, 9999}, // 9.999,9 dibulatkan ke bawah
		{99, 1, 0},        // 0,99 dibulatkan ke bawah
		{175000, 33, 57750},
		{150000, 100, 150000}, // gratis
		{0, 50, 0},            // tiket gratis tetap gratis
	}
	for _, k := range kasus {
		if got := HitungPotongan(k.harga, k.persen); got != k.mau {
			t.Errorf("HitungPotongan(%d, %d%%) = %d, mau %d", k.harga, k.persen, got, k.mau)
		}
	}
}

func TestBerlakuUntukKategoriTerdaftar(t *testing.T) {
	p := Promo{KategoriID: []int64{2, 5}}
	if !p.BerlakuUntuk(2) || !p.BerlakuUntuk(5) {
		t.Error("kategori terdaftar tidak mendapat potongan")
	}
	if p.BerlakuUntuk(1) {
		t.Error("kategori tidak terdaftar mendapat potongan")
	}
}

// ===== BR-16 =====

func TestMasihAdaKuota(t *testing.T) {
	if !(Promo{KuotaPemakaian: 2, Terpakai: 1}).MasihAdaKuota() {
		t.Error("terpakai 1 dari 2 dianggap habis")
	}
	if (Promo{KuotaPemakaian: 2, Terpakai: 2}).MasihAdaKuota() {
		t.Error("terpakai 2 dari 2 dianggap masih ada")
	}
}
