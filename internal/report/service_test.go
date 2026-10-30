package report

import "testing"

func TestBersihBR28(t *testing.T) {
	a := Angka{Kotor: 500000, Potongan: 22500, Refund: 95625}
	if got := a.Bersih(); got != 381875 {
		t.Errorf("bersih %d, mau 381875 (500.000 - 22.500 - 95.625)", got)
	}
	if got := (Angka{}).Bersih(); got != 0 {
		t.Errorf("tanpa penjualan: bersih %d, mau 0", got)
	}
}

func TestSusunLaporanTahapKategoriTotal(t *testing.T) {
	baris := []BarisTahap{
		{KategoriID: 1, NamaKategori: "Festival", TahapID: 10, Angka: Angka{Terjual: 3, Kotor: 300000, Potongan: 15000}},
		{KategoriID: 1, NamaKategori: "Festival", TahapID: 11, Angka: Angka{Terjual: 2, TiketRefund: 1, Kotor: 300000, Refund: 112500}},
		{KategoriID: 2, NamaKategori: "VIP", TahapID: 20}, // belum terjual
	}
	kategori, total := SusunLaporan(baris)

	if len(kategori) != 2 || len(kategori[0].Tahap) != 2 || len(kategori[1].Tahap) != 1 {
		t.Fatalf("pengelompokan salah: %+v", kategori)
	}
	festival := kategori[0].Subtotal
	if festival.Terjual != 5 || festival.Kotor != 600000 || festival.Potongan != 15000 || festival.Refund != 112500 || festival.TiketRefund != 1 {
		t.Errorf("subtotal Festival %+v", festival)
	}
	if kategori[1].Subtotal != (Angka{}) {
		t.Errorf("subtotal VIP harus nol: %+v", kategori[1].Subtotal)
	}

	// invarian: jumlah per tahap = jumlah per kategori = total event
	var dariTahap, dariKategori Angka
	for _, b := range baris {
		dariTahap.Tambah(b.Angka)
	}
	for _, k := range kategori {
		dariKategori.Tambah(k.Subtotal)
	}
	if dariTahap != total || dariKategori != total {
		t.Errorf("tahap %+v, kategori %+v, total %+v harus sama", dariTahap, dariKategori, total)
	}
	if total.Bersih() != 472500 {
		t.Errorf("bersih total %d, mau 472500", total.Bersih())
	}
}

func TestSusunLaporanTanpaBaris(t *testing.T) {
	kategori, total := SusunLaporan(nil)
	if len(kategori) != 0 || total != (Angka{}) {
		t.Errorf("tanpa baris: %+v, %+v", kategori, total)
	}
}
