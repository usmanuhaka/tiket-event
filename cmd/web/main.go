package main

import (
	"context"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
	_ "time/tzdata" // data zona waktu ikut di dalam program, supaya Asia/Jakarta selalu dikenali

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

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

	log.Println("server berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func homeHandler(tpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		web.Render(w, tpl, nil)
	}
}
