// Package store persists Dashboard state in a SQLite database inside the data directory.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DBFile is the database file name inside the data directory.
const DBFile = "telltale.db"

const settingAdminPasswordHash = "admin_password_hash"

// migrations are applied in order; the database's user_version records how many have run.
var migrations = []string{
	`CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE sessions (
		token_hash BLOB PRIMARY KEY,
		expires_at INTEGER NOT NULL
	);`,
	`CREATE TABLE hosts (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		token_hash BLOB NOT NULL UNIQUE,
		created_at INTEGER NOT NULL
	);`,
	`ALTER TABLE hosts ADD COLUMN host_info TEXT;`,
	`ALTER TABLE hosts ADD COLUMN ipv4 TEXT NOT NULL DEFAULT '';
	ALTER TABLE hosts ADD COLUMN ipv4_seen_at INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE hosts ADD COLUMN ipv6 TEXT NOT NULL DEFAULT '';
	ALTER TABLE hosts ADD COLUMN ipv6_seen_at INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE hosts ADD COLUMN country_override TEXT NOT NULL DEFAULT '';`,
}

// Store is the Dashboard's persistent state.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database in dataDir and applies migrations.
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	dsn := "file:" + filepath.Join(dataDir, DBFile) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this Dashboard supports (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// AdminPasswordHash returns the stored admin password hash, or "" if none is set.
func (s *Store) AdminPasswordHash(ctx context.Context) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingAdminPasswordHash).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return hash, err
}

// SetAdminPasswordHash stores the admin password hash and ends every existing session.
func (s *Store) SetAdminPasswordHash(ctx context.Context, hash string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		settingAdminPasswordHash, hash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateSession records a session by the hash of its token.
func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, expires_at) VALUES (?, ?)`,
		tokenHash, expiresAt.Unix())
	return err
}

// SessionValid reports whether a session with this token hash exists and has not expired at now.
func (s *Store) SessionValid(ctx context.Context, tokenHash []byte, now time.Time) (bool, error) {
	var expiresAt int64
	err := s.db.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash = ?`, tokenHash).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return now.Unix() < expiresAt, nil
}

// DeleteSession ends a session.
func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// DeleteExpiredSessions removes sessions that have expired at now.
func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now.Unix())
	return err
}

// Host is a machine registered in the Dashboard.
type Host struct {
	ID string
	// Name is the display name; empty means "use the Host's hostname".
	Name      string
	CreatedAt time.Time
	// HostInfo is the latest Host Info as JSON, or nil if none has arrived.
	HostInfo []byte
	// IPv4 and IPv6 are the source addresses of the Agent's last Host Info
	// over each IP family (zero if never seen), and when each was last seen.
	IPv4, IPv6             netip.Addr
	IPv4SeenAt, IPv6SeenAt time.Time
	// CountryOverride is the admin's manual ISO country code, or "".
	CountryOverride string
}

// CreateHost registers a Host with the hash of its token.
func (s *Store) CreateHost(ctx context.Context, h Host, tokenHash []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO hosts (id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`,
		h.ID, h.Name, tokenHash, h.CreatedAt.Unix())
	return err
}

const hostColumns = `id, name, created_at, host_info, ipv4, ipv4_seen_at, ipv6, ipv6_seen_at, country_override`

func scanHost(scan func(...any) error) (Host, error) {
	var h Host
	var created, v4Seen, v6Seen int64
	var info sql.NullString
	var v4, v6 string
	if err := scan(&h.ID, &h.Name, &created, &info, &v4, &v4Seen, &v6, &v6Seen, &h.CountryOverride); err != nil {
		return Host{}, err
	}
	h.CreatedAt = time.Unix(created, 0)
	if info.Valid {
		h.HostInfo = []byte(info.String)
	}
	h.IPv4, _ = netip.ParseAddr(v4)
	h.IPv6, _ = netip.ParseAddr(v6)
	h.IPv4SeenAt, h.IPv6SeenAt = time.Unix(v4Seen, 0), time.Unix(v6Seen, 0)
	return h, nil
}

// Hosts returns all Hosts in the order they were added.
func (s *Store) Hosts(ctx context.Context) ([]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+hostColumns+` FROM hosts ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []Host
	for rows.Next() {
		h, err := scanHost(rows.Scan)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

// HostByTokenHash returns the Host whose token has this hash.
func (s *Store) HostByTokenHash(ctx context.Context, tokenHash []byte) (Host, bool, error) {
	h, err := scanHost(s.db.QueryRowContext(ctx, `SELECT `+hostColumns+` FROM hosts WHERE token_hash = ?`, tokenHash).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Host{}, false, nil
	}
	if err != nil {
		return Host{}, false, err
	}
	return h, true, nil
}

// SetHostInfo stores a Host's latest Host Info (as JSON) and the address it
// arrived from, as that Host's IPv4 or IPv6 address.
func (s *Store) SetHostInfo(ctx context.Context, hostID string, info []byte, from netip.Addr, at time.Time) error {
	from = from.Unmap()
	switch {
	case from.Is4():
		_, err := s.db.ExecContext(ctx, `UPDATE hosts SET host_info = ?, ipv4 = ?, ipv4_seen_at = ? WHERE id = ?`,
			string(info), from.String(), at.Unix(), hostID)
		return err
	case from.Is6():
		_, err := s.db.ExecContext(ctx, `UPDATE hosts SET host_info = ?, ipv6 = ?, ipv6_seen_at = ? WHERE id = ?`,
			string(info), from.String(), at.Unix(), hostID)
		return err
	default:
		_, err := s.db.ExecContext(ctx, `UPDATE hosts SET host_info = ? WHERE id = ?`, string(info), hostID)
		return err
	}
}

// SetCountryOverride sets (or, with "", clears) a Host's manual country.
// It reports whether the Host exists.
func (s *Store) SetCountryOverride(ctx context.Context, hostID, country string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE hosts SET country_override = ? WHERE id = ?`, country, hostID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
