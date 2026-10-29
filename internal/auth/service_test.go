package auth

import (
	"strings"
	"testing"
)

func TestValidasiDaftar(t *testing.T) {
	valid := DataDaftar{Nama: "Budi", Email: "budi@kampus.test", Password: "rahasia123", Konfirmasi: "rahasia123"}

	kasus := []struct {
		nama     string
		ubah     func(d *DataDaftar)
		mauLulus bool
	}{
		{"semua valid", func(d *DataDaftar) {}, true},
		{"email huruf besar dan spasi tetap valid", func(d *DataDaftar) { d.Email = "  Budi@Kampus.TEST " }, true},
		{"nama kosong", func(d *DataDaftar) { d.Nama = "   " }, false},
		{"email tanpa @", func(d *DataDaftar) { d.Email = "budi.kampus.test" }, false},
		{"email domain tanpa titik", func(d *DataDaftar) { d.Email = "budi@kampus" }, false},
		{"email dengan spasi", func(d *DataDaftar) { d.Email = "bu di@kampus.test" }, false},
		{"password 7 karakter", func(d *DataDaftar) { d.Password, d.Konfirmasi = "1234567", "1234567" }, false},
		{"password lebih dari 72 byte", func(d *DataDaftar) {
			p := strings.Repeat("a", 73)
			d.Password, d.Konfirmasi = p, p
		}, false},
		{"konfirmasi beda", func(d *DataDaftar) { d.Konfirmasi = "rahasia124" }, false},
	}

	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			d := valid
			k.ubah(&d)
			err := ValidasiDaftar(d)
			if k.mauLulus && err != nil {
				t.Errorf("mau lulus, dapat error: %v", err)
			}
			if !k.mauLulus && err == nil {
				t.Errorf("mau ditolak, tapi lulus")
			}
		})
	}
}

func TestNormalisasiEmail(t *testing.T) {
	if got := NormalisasiEmail("  Budi@Kampus.TEST "); got != "budi@kampus.test" {
		t.Errorf("dapat %q", got)
	}
}

func TestHashDanCekPassword(t *testing.T) {
	hash, err := HashPassword("rahasia123")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "rahasia123" {
		t.Fatal("password tersimpan apa adanya, bukan hash")
	}
	if !CekPassword(hash, "rahasia123") {
		t.Error("password benar ditolak")
	}
	if CekPassword(hash, "rahasia124") {
		t.Error("password salah diterima")
	}
}

func TestBuatToken(t *testing.T) {
	a := BuatToken()
	b := BuatToken()
	if len(a) != 64 {
		t.Errorf("panjang token %d, mau 64", len(a))
	}
	if a == b {
		t.Error("dua token sama, seharusnya acak")
	}
}

func TestBolehAkses(t *testing.T) {
	if !BolehAkses(PeranAdmin, PeranAdmin) {
		t.Error("admin ditolak di halaman admin")
	}
	if BolehAkses(PeranPembeli, PeranAdmin, PeranPetugas) {
		t.Error("pembeli diterima di halaman admin/petugas")
	}
	if BolehAkses("", PeranPembeli) {
		t.Error("tamu (peran kosong) diterima")
	}
}
