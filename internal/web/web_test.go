package web

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRupiah(t *testing.T) {
	kasus := []struct {
		angka int64
		mau   string
	}{
		{0, "Rp0"},
		{500, "Rp500"},
		{1000, "Rp1.000"},
		{150000, "Rp150.000"},
		{39000000, "Rp39.000.000"},
		{-75000, "Rp-75.000"},
	}
	for _, k := range kasus {
		if got := Rupiah(k.angka); got != k.mau {
			t.Errorf("Rupiah(%d) = %q, mau %q", k.angka, got, k.mau)
		}
	}
}

func TestRenderStatus(t *testing.T) {
	tpl := template.Must(template.New("layout").Parse(`{{define "layout"}}{{.Error}}{{end}}`))

	w := httptest.NewRecorder()
	RenderStatus(w, tpl, Halaman{Error: "Email atau password salah"}, http.StatusUnauthorized)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, mau %d", w.Code, http.StatusUnauthorized)
	}
	if w.Body.String() != "Email atau password salah" {
		t.Errorf("isi = %q", w.Body.String())
	}
}

func TestRenderGagalBalas500(t *testing.T) {
	// template "layout" memanggil field yang tidak ada, jadi eksekusi gagal
	tpl := template.Must(template.New("layout").Parse(`{{define "layout"}}{{.TidakAda}}{{end}}`))

	w := httptest.NewRecorder()
	Render(w, tpl, struct{}{})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, mau %d", w.Code, http.StatusInternalServerError)
	}
}
