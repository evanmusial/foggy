package foggy

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mutecomm/go-sqlcipher/v4"
)

func (a *App) openEncryptedDB(dbKey []byte) error {
	if err := os.MkdirAll(a.dataDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(a.attachments, 0o700); err != nil {
		return err
	}
	dsn := fmt.Sprintf("%s?_pragma_key=x'%s'&_pragma_cipher_page_size=4096&_busy_timeout=5000", a.dbPath, hexDBKey(dbKey))
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA temp_store = MEMORY; PRAGMA journal_mode = WAL;`); err != nil {
		_ = db.Close()
		return err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return err
	}
	a.dbMu.Lock()
	old := a.db
	a.db = db
	a.dbKey = dbKey
	a.dbMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (a *App) dbConn() (*sql.DB, error) {
	a.dbMu.RLock()
	db := a.db
	a.dbMu.RUnlock()
	if db == nil {
		return nil, fmt.Errorf("database is locked")
	}
	return db, nil
}

func (a *App) currentDBKey() ([]byte, error) {
	a.dbMu.RLock()
	defer a.dbMu.RUnlock()
	if a.dbKey == nil {
		return nil, fmt.Errorf("database is locked")
	}
	return a.dbKey, nil
}

func (a *App) unlockWithPassword(password string) error {
	a.securityMu.RLock()
	state := a.security
	a.securityMu.RUnlock()
	if !state.PasswordAuthEnabled {
		return fmt.Errorf("password authentication is disabled")
	}
	if !verifyPasswordState(state, password) {
		return fmt.Errorf("invalid login")
	}
	dbKey, err := unwrapWithPassword(state.DBKeyWrapped, password)
	if err != nil {
		return err
	}
	return a.openEncryptedDB(dbKey)
}

func (a *App) unlockForConvenience() error {
	a.securityMu.RLock()
	profile := a.security.EncryptionProfile
	a.securityMu.RUnlock()
	if profile != profileConvenience {
		return fmt.Errorf("password required after restart")
	}
	dbKey, err := a.unlockWithServerKey()
	if err != nil {
		return err
	}
	return a.openEncryptedDB(dbKey)
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS app_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS user_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			theme TEXT NOT NULL DEFAULT 'light',
			accent_color TEXT NOT NULL DEFAULT '#2254ab',
			font_scale TEXT NOT NULL DEFAULT 'comfortable',
			high_contrast INTEGER NOT NULL DEFAULT 0,
			reduced_motion INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS daily_checkins (
			id TEXT PRIMARY KEY,
			entry_date TEXT NOT NULL UNIQUE,
			overall_burden INTEGER NOT NULL,
			fatigue INTEGER NOT NULL,
			energy INTEGER NOT NULL,
			pain INTEGER NOT NULL,
			mood INTEGER NOT NULL,
			anxiety INTEGER NOT NULL,
			brain_fog INTEGER NOT NULL,
			sleep_quality INTEGER NOT NULL,
			heat_sensitivity INTEGER NOT NULL,
			mobility INTEGER NOT NULL,
			bladder_bowel INTEGER NOT NULL,
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS symptom_events (
			id TEXT PRIMARY KEY,
			occurred_at TEXT NOT NULL,
			category TEXT NOT NULL,
			symptom TEXT NOT NULL,
			body_location TEXT NOT NULL DEFAULT '',
			severity INTEGER NOT NULL,
			duration TEXT NOT NULL DEFAULT '',
			newness TEXT NOT NULL DEFAULT '',
			function_impact TEXT NOT NULL DEFAULT '',
			heat_exposure TEXT NOT NULL DEFAULT '',
			triggers_json TEXT NOT NULL DEFAULT '[]',
			infection_signs TEXT NOT NULL DEFAULT '',
			body_temperature TEXT NOT NULL DEFAULT '',
			treatment_response TEXT NOT NULL DEFAULT '',
			relapse_flag TEXT NOT NULL DEFAULT 'uncertain',
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS symptom_events_occurred_idx ON symptom_events(occurred_at);`,
		`CREATE TABLE IF NOT EXISTS medication_events (
			id TEXT PRIMARY KEY,
			taken_at TEXT NOT NULL,
			name TEXT NOT NULL,
			dose TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			effectiveness TEXT NOT NULL DEFAULT '',
			side_effects TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS episodes (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			relapse_flag TEXT NOT NULL DEFAULT 'uncertain',
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tags (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE
		);`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			related_type TEXT NOT NULL,
			related_id TEXT NOT NULL,
			filename TEXT NOT NULL,
			content_type TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			storage_path TEXT NOT NULL,
			nonce TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			occurred_at TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}'
		);`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	_, err := db.Exec(`INSERT OR IGNORE INTO user_settings(id, theme, accent_color, font_scale, high_contrast, reduced_motion, updated_at)
		VALUES(1, 'light', '#2254ab', 'comfortable', 0, 0, ?)`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (a *App) audit(eventType string, metadata map[string]any) {
	db, err := a.dbConn()
	if err != nil {
		return
	}
	id, err := randomID("audit")
	if err != nil {
		return
	}
	b, _ := json.Marshal(metadata)
	_, _ = db.Exec(`INSERT INTO audit_events(id, event_type, occurred_at, metadata_json) VALUES(?,?,?,?)`,
		id, eventType, time.Now().UTC().Format(time.RFC3339), string(b))
}

func encodeNonce(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeNonce(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func safeFilename(name string) string {
	name = filepath.Base(name)
	if name == "." || name == "/" || name == "" {
		return "attachment.bin"
	}
	return url.PathEscape(name)
}
