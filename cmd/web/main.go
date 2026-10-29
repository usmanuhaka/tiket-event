package main

import (
	"context"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	"github.com/usmanuhaka/tiket-event/internal/auth"
	"github.com/usmanuhaka/tiket-event/internal/event"
	"github.com/usmanuhaka/tiket-event/internal/order"
	"github.com/usmanuhaka/tiket-event/internal/web"
)

func main() {
	_ = godotenv.Load() // baca .env jika ada

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		log.Fatal("DB_DSN belum di-set (cek file .env)")
	}

	db, err := sql.Open("mysql", dsn) // db bertipe *sql.DB
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("database tidak bisa dihubungi: %v", err)
	}
	log.Println("database terhubung")

	tplHome := web.ParseHalaman("home.html")

	mux := http.NewServeMux()

	// "/{$}" hanya cocok dengan "/" persis; alamat lain yang tidak terdaftar mendapat 404
	mux.HandleFunc("GET /{$}", homeHandler(tplHome))

	// login, daftar, logout
	auth.RegisterRoutes(mux, db)

	// pengelolaan event, kategori, tahap harga (khusus ADMIN)
	event.RegisterRoutes(mux, db)

	// katalog event, pemesanan, dan pembayaran (pembeli)
	order.RegisterRoutes(mux, db)

	// BR-10: worker berjalan di latar belakang (goroutine), memeriksa pesanan kedaluwarsa tiap 1 menit
	go order.JalankanWorker(context.Background(), db, time.Minute)

	// MuatPengguna membungkus semua route: setiap request dicek cookie sesinya dulu
	app := auth.MuatPengguna(db, mux)

	log.Println("server berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", app))
}

func homeHandler(tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		web.Render(w, tpl, auth.Halaman(r))
	}
}
