// Package web berisi helper tampilan yang dipakai semua fitur:
// format rupiah, memuat template halaman, dan merender halaman.
package web

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"strconv"
)

// Rupiah memformat angka jadi "Rp39.000.000"
func Rupiah(n int64) string {
	s := strconv.FormatInt(n, 10)
	hasil := ""
	for i, c := range s {
		// sisipkan titik tiap 3 digit dari kanan, kecuali setelah tanda minus
		if i > 0 && (len(s)-i)%3 == 0 && s[i-1] != '-' {
			hasil += "."
		}
		hasil += string(c)
	}
	return "Rp" + hasil
}

// AddOne dipakai di template untuk nomor urut (index range dimulai dari 0)
func AddOne(n int) int {
	return n + 1
}

var funcs = template.FuncMap{"rupiah": Rupiah, "add_one": AddOne}

// ParseHalaman memuat layout + header + sidebar + footer + satu file halaman.
// Path relatif terhadap folder proyek, jadi server harus dijalankan dari root proyek.
func ParseHalaman(file string) *template.Template {
	return template.Must(template.New("layout").Funcs(funcs).ParseFiles(
		"templates/layout.html",
		"templates/header.html",
		"templates/sidebar.html",
		"templates/footer.html",
		"templates/"+file,
	))
}

// Halaman berisi data yang dibutuhkan layout (header, sidebar, kotak pesan) di setiap halaman.
// Data tiap halaman menyertakan struct ini (embedding), sehingga di template
// cukup ditulis {{.Nama}} atau {{.Peran}}.
type Halaman struct {
	Nama  string // nama pengguna yang login, kosong jika tamu
	Peran string // ADMIN, PEMBELI, PETUGAS, atau kosong jika tamu
	Pesan string // pesan berhasil (kotak hijau)
	Error string // pesan gagal (kotak merah)
}

// DataPesan dipakai templates/pesan.html untuk halaman pemberitahuan sederhana (mis. 403)
type DataPesan struct {
	Halaman
	Judul string
	Isi   string
}

// Render menampilkan halaman lewat template "layout" dengan status 200 OK.
func Render(w http.ResponseWriter, tpl *template.Template, data any) {
	RenderStatus(w, tpl, data, http.StatusOK)
}

// RenderStatus sama seperti Render, tetapi dengan kode status tertentu
// (mis. 400 jika isi form salah, 401 jika login gagal, 403 jika tidak punya akses).
// Hasil ditulis ke buffer dulu: jika template gagal di tengah jalan,
// pengguna mendapat pesan error yang utuh, bukan halaman setengah jadi.
// Error tidak mematikan server (berbeda dengan log.Fatal).
func RenderStatus(w http.ResponseWriter, tpl *template.Template, data any, status int) {
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("gagal render template: %v", err)
		http.Error(w, "Terjadi kesalahan saat menampilkan halaman", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(status)
	buf.WriteTo(w)
}
