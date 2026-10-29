package ticket

import (
	"strings"
	"testing"
)

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
