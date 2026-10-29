package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// User adalah satu baris tabel users
type User struct {
	ID           int64
	Nama         string
	Email        string
	PasswordHash string
	Role         string
}

// ErrEmailTerdaftar dikembalikan simpanUser jika email sudah dipakai akun lain
var ErrEmailTerdaftar = errors.New("Email sudah terdaftar")

// kodeDuplikat adalah nomor error MySQL/MariaDB untuk pelanggaran UNIQUE
const kodeDuplikat = 1062

func getUserByEmail(ctx context.Context, db *sql.DB, email string) (User, error) {
	var u User

	err := db.QueryRowContext(ctx,
		"SELECT id, nama, email, password_hash, role FROM users WHERE email = ? AND deleted_at IS NULL", email,
	).Scan(&u.ID, &u.Nama, &u.Email, &u.PasswordHash, &u.Role)

	return u, err
}

func simpanUser(ctx context.Context, db *sql.DB, u User) (int64, error) {
	res, err := db.ExecContext(ctx,
		"INSERT INTO users (nama, email, password_hash, role) VALUES (?, ?, ?, ?)",
		u.Nama, u.Email, u.PasswordHash, u.Role,
	)
	if err != nil {
		// kolom email UNIQUE: jika dua orang mendaftar dengan email sama, database yang menolak
		var errMysql *mysql.MySQLError
		if errors.As(err, &errMysql) && errMysql.Number == kodeDuplikat {
			return 0, ErrEmailTerdaftar
		}
		return 0, err
	}
	id, err := res.LastInsertId()

	return id, err
}

func simpanSession(ctx context.Context, db *sql.DB, token string, userID int64, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO sessions (token, user_id, expires_at) VALUES (?, ?, ?)",
		token, userID, expiresAt,
	)
	return err
}

// getUserBySessionToken mencari pemilik token. Sesi yang sudah kedaluwarsa
// atau milik user yang sudah dihapus dianggap tidak ada (sql.ErrNoRows).
func getUserBySessionToken(ctx context.Context, db *sql.DB, token string, now time.Time) (User, error) {
	var u User

	err := db.QueryRowContext(ctx,
		`SELECT u.id, u.nama, u.email, u.password_hash, u.role
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token = ? AND s.expires_at > ? AND u.deleted_at IS NULL`,
		token, now,
	).Scan(&u.ID, &u.Nama, &u.Email, &u.PasswordHash, &u.Role)

	return u, err
}

// hapusSession dipakai saat logout. Sesi dihapus sungguhan (bukan soft delete)
// karena hanya data teknis login, bukan data bisnis.
func hapusSession(ctx context.Context, db *sql.DB, token string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	return err
}
