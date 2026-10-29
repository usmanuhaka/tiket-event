package event

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/usmanuhaka/tiket-event/internal/testdb"
)

// bukaDBKosong membuka database test dengan semua tabel kosong
func bukaDBKosong(t *testing.T) *sql.DB {
	db := testdb.Buka(t)
	testdb.KosongkanSemua(t, db)
	return db
}

// waktuSekarang tanpa detik pecahan, supaya sama persis dengan kolom DATETIME
func waktuSekarang() time.Time {
	return time.Now().Truncate(time.Second)
}

// buatEventDraft menyimpan event DRAFT 30 hari lagi dan mengembalikan id-nya
func buatEventDraft(t *testing.T, db *sql.DB, kapasitas int) int64 {
	t.Helper()
	id, err := simpanEvent(context.Background(), db, Event{
		Nama: "Konser Test", Lokasi: "Aula", Kapasitas: kapasitas,
		TanggalEvent: waktuSekarang().Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func harusTambahKategori(t *testing.T, db *sql.DB, eventID int64, nama string, kuota int) int64 {
	t.Helper()
	id, err := tambahKategori(context.Background(), db, eventID, nama, kuota)
	if err != nil {
		t.Fatalf("tambah kategori %s: %v", nama, err)
	}
	return id
}

func harusTambahTahap(t *testing.T, db *sql.DB, eventID, kategoriID int64, nama string, kuota, hariLagi int) int64 {
	t.Helper()
	id, err := tambahTahap(context.Background(), db, eventID, kategoriID, Tahap{
		Nama: nama, Harga: 100000, Kuota: kuota,
		BatasTanggal: waktuSekarang().Add(time.Duration(hariLagi) * 24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("tambah tahap %s: %v", nama, err)
	}
	return id
}

func statusEvent(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	e, err := GetEventById(context.Background(), db, id)
	if err != nil {
		t.Fatal(err)
	}
	return e.Status
}

func TestBR01EventPunyaBeberapaKategori(t *testing.T) {
	db := bukaDBKosong(t)
	eventID := buatEventDraft(t, db, 500)
	harusTambahKategori(t, db, eventID, "Festival", 400)
	harusTambahKategori(t, db, eventID, "VIP", 100)

	kategori, err := AmbilKategoriDanTahap(context.Background(), db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if len(kategori) != 2 || kategori[0].Kuota != 400 || kategori[1].Kuota != 100 {
		t.Errorf("dapat %+v", kategori)
	}
}

func TestBR02KategoriMelebihiKapasitasDitolak(t *testing.T) {
	db := bukaDBKosong(t)
	eventID := buatEventDraft(t, db, 500)
	harusTambahKategori(t, db, eventID, "Festival", 400)

	_, err := tambahKategori(context.Background(), db, eventID, "VIP", 101)
	var salah KesalahanInput
	if !errors.As(err, &salah) {
		t.Fatalf("mau KesalahanInput, dapat %v", err)
	}

	kategori, _ := AmbilKategoriDanTahap(context.Background(), db, eventID)
	if len(kategori) != 1 {
		t.Errorf("kategori yang ditolak tetap tersimpan, jumlah %d", len(kategori))
	}
}

// Dua puluh request menambah kategori 20 kursi secara bersamaan ke event berkapasitas 100.
// Tanpa FOR UPDATE, banyak request bisa membaca total lama yang sama lalu sama-sama lolos.
// Dengan FOR UPDATE, request antre satu per satu: tepat 5 yang berhasil.
func TestBR02TambahKategoriBersamaanTidakMelebihiKapasitas(t *testing.T) {
	db := bukaDBKosong(t)
	eventID := buatEventDraft(t, db, 100)

	var wg sync.WaitGroup
	var mu sync.Mutex
	berhasil := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := tambahKategori(context.Background(), db, eventID, "Kategori", 20)
			if err == nil {
				mu.Lock()
				berhasil++
				mu.Unlock()
				return
			}
			var salah KesalahanInput
			if !errors.As(err, &salah) {
				t.Errorf("error selain penolakan kuota: %v", err)
			}
		}()
	}
	wg.Wait()

	if berhasil != 5 {
		t.Errorf("berhasil %d kategori, mau 5", berhasil)
	}
	var total int
	db.QueryRow("SELECT SUM(kuota) FROM ticket_categories WHERE event_id = ?", eventID).Scan(&total)
	if total != 100 {
		t.Errorf("total kuota kategori %d, mau 100", total)
	}
}

func TestBR02KapasitasTidakBolehDiturunkanDiBawahTotalKategori(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID := buatEventDraft(t, db, 500)
	harusTambahKategori(t, db, eventID, "Festival", 400)

	e, _ := GetEventById(ctx, db, eventID)
	e.Kapasitas = 399
	if err := updateEvent(ctx, db, e); err == nil {
		t.Error("kapasitas 399 diterima padahal total kategori 400")
	}
	e.Kapasitas = 400
	if err := updateEvent(ctx, db, e); err != nil {
		t.Errorf("kapasitas 400 ditolak: %v", err)
	}
}

func TestBR03UrutanTahapOtomatis(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID := buatEventDraft(t, db, 100)
	kategoriID := harusTambahKategori(t, db, eventID, "Festival", 100)

	harusTambahTahap(t, db, eventID, kategoriID, "Presale 1", 20, 5)
	presale2 := harusTambahTahap(t, db, eventID, kategoriID, "Presale 2", 20, 10)
	harusTambahTahap(t, db, eventID, kategoriID, "Normal", 20, 20)

	// hapus Presale 2 lalu tambah tahap baru: urutannya 4, bukan bentrok dengan 2 yang sudah dihapus
	if err := hapusTahap(ctx, db, eventID, presale2); err != nil {
		t.Fatal(err)
	}
	harusTambahTahap(t, db, eventID, kategoriID, "Presale Akhir", 40, 25)

	kategori, err := AmbilKategoriDanTahap(ctx, db, eventID)
	if err != nil {
		t.Fatal(err)
	}
	tahap := kategori[0].Tahap
	mau := []struct {
		nama   string
		urutan int
	}{{"Presale 1", 1}, {"Normal", 3}, {"Presale Akhir", 4}}
	if len(tahap) != len(mau) {
		t.Fatalf("jumlah tahap %d, mau %d", len(tahap), len(mau))
	}
	for i, m := range mau {
		if tahap[i].Nama != m.nama || tahap[i].Urutan != m.urutan {
			t.Errorf("tahap ke-%d: dapat %s/%d, mau %s/%d", i, tahap[i].Nama, tahap[i].Urutan, m.nama, m.urutan)
		}
		if tahap[i].SisaKuota != tahap[i].Kuota {
			t.Errorf("sisa kuota awal %s harus sama dengan kuota", tahap[i].Nama)
		}
	}
}

func TestBR04TahapMelebihiKuotaKategoriDitolak(t *testing.T) {
	db := bukaDBKosong(t)
	eventID := buatEventDraft(t, db, 100)
	kategoriID := harusTambahKategori(t, db, eventID, "Festival", 100)
	harusTambahTahap(t, db, eventID, kategoriID, "Presale 1", 60, 5)

	_, err := tambahTahap(context.Background(), db, eventID, kategoriID, Tahap{
		Nama: "Normal", Harga: 100000, Kuota: 41, BatasTanggal: waktuSekarang().Add(24 * time.Hour),
	})
	var salah KesalahanInput
	if !errors.As(err, &salah) {
		t.Errorf("mau KesalahanInput, dapat %v", err)
	}
}

func TestBukaPenjualanBerhasil(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID := buatEventDraft(t, db, 100)
	kategoriID := harusTambahKategori(t, db, eventID, "Festival", 100)
	harusTambahTahap(t, db, eventID, kategoriID, "Presale 1", 40, 5)
	harusTambahTahap(t, db, eventID, kategoriID, "Normal", 60, 20)

	masalah, err := bukaPenjualan(ctx, db, eventID, time.Now())
	if err != nil || len(masalah) != 0 {
		t.Fatalf("buka penjualan gagal: err=%v masalah=%v", err, masalah)
	}
	if s := statusEvent(t, db, eventID); s != StatusOpen {
		t.Errorf("status %s, mau OPEN", s)
	}
}

func TestBukaPenjualanGagalTetapDraft(t *testing.T) {
	db := bukaDBKosong(t)
	eventID := buatEventDraft(t, db, 100)
	kategoriID := harusTambahKategori(t, db, eventID, "Festival", 100)
	harusTambahTahap(t, db, eventID, kategoriID, "Presale 1", 40, 5) // baru 40 dari 100 (BR-04)

	masalah, err := bukaPenjualan(context.Background(), db, eventID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(masalah) == 0 {
		t.Error("buka penjualan lolos padahal total tahap belum sama dengan kuota kategori")
	}
	if s := statusEvent(t, db, eventID); s != StatusDraft {
		t.Errorf("status %s, mau tetap DRAFT", s)
	}
}

func TestEventOpenTidakBisaDiubah(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID := buatEventDraft(t, db, 100)
	kategoriID := harusTambahKategori(t, db, eventID, "Festival", 100)
	tahapID := harusTambahTahap(t, db, eventID, kategoriID, "Normal", 100, 20)
	if _, err := bukaPenjualan(ctx, db, eventID, time.Now()); err != nil {
		t.Fatal(err)
	}

	e, _ := GetEventById(ctx, db, eventID)
	_, errKategori := tambahKategori(ctx, db, eventID, "VIP", 1)
	_, errTahap := tambahTahap(ctx, db, eventID, kategoriID, Tahap{Nama: "X", Kuota: 1, BatasTanggal: time.Now()})

	semua := map[string]error{
		"update event":    updateEvent(ctx, db, e),
		"hapus event":     hapusEvent(ctx, db, eventID),
		"tambah kategori": errKategori,
		"hapus kategori":  hapusKategori(ctx, db, eventID, kategoriID),
		"tambah tahap":    errTahap,
		"hapus tahap":     hapusTahap(ctx, db, eventID, tahapID),
	}
	for aksi, err := range semua {
		if !errors.Is(err, ErrBukanDraft) {
			t.Errorf("%s pada event OPEN: mau ErrBukanDraft, dapat %v", aksi, err)
		}
	}
}

func TestHapusKategoriIkutHapusTahap(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID := buatEventDraft(t, db, 100)
	kategoriID := harusTambahKategori(t, db, eventID, "Festival", 100)
	harusTambahTahap(t, db, eventID, kategoriID, "Normal", 100, 20)

	if err := hapusKategori(ctx, db, eventID, kategoriID); err != nil {
		t.Fatal(err)
	}

	var tahapAktif int
	db.QueryRow("SELECT COUNT(*) FROM price_tiers WHERE category_id = ? AND deleted_at IS NULL", kategoriID).Scan(&tahapAktif)
	if tahapAktif != 0 {
		t.Errorf("masih ada %d tahap yang belum terhapus", tahapAktif)
	}

	// kuota yang dibebaskan bisa dipakai kategori baru
	harusTambahKategori(t, db, eventID, "Festival Baru", 100)
}

func TestDataTidakDitemukan(t *testing.T) {
	db := bukaDBKosong(t)
	ctx := context.Background()
	eventID := buatEventDraft(t, db, 100)
	eventLain := buatEventDraft(t, db, 100)
	kategoriLain := harusTambahKategori(t, db, eventLain, "Milik event lain", 50)

	if _, err := GetEventById(ctx, db, 999999); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("event tidak ada: dapat %v", err)
	}
	if _, err := tambahKategori(ctx, db, 999999, "VIP", 1); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("tambah kategori ke event tidak ada: dapat %v", err)
	}
	// kategori milik event lain tidak bisa dihapus lewat URL event ini
	if err := hapusKategori(ctx, db, eventID, kategoriLain); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("hapus kategori event lain: dapat %v", err)
	}
	if _, err := tambahTahap(ctx, db, eventID, kategoriLain, Tahap{Nama: "X", Kuota: 1, BatasTanggal: time.Now()}); !errors.Is(err, ErrTidakDitemukan) {
		t.Errorf("tambah tahap ke kategori event lain: dapat %v", err)
	}
}
