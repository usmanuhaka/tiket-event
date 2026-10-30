package ticket

import "crypto/rand"

// hurufKode berisi 32 huruf dan angka tanpa 0/O dan 1/I, supaya kode tidak salah baca di pintu masuk
const hurufKode = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// BuatKodeTiket (BR-11, BR-22) membuat kode tiket acak, mis. "TKT-7K3M9QX2".
// 8 karakter dari 32 pilihan = sekitar 1 triliun kemungkinan.
// Dipakai saat pembayaran (paket order) dan saat pemindahan tiket (BR-22).
// Keunikan tetap dijaga kolom UNIQUE di database.
func BuatKodeTiket() string {
	b := make([]byte, 8)
	rand.Read(b) // crypto/rand: tidak bisa ditebak; sejak Go 1.24 tidak pernah error
	kode := make([]byte, 8)
	for i := range b {
		// 256 habis dibagi 32, jadi setiap huruf punya peluang yang sama
		kode[i] = hurufKode[int(b[i])%len(hurufKode)]
	}
	return "TKT-" + string(kode)
}
