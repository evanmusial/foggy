package foggy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPasswordLoginDoesNotOpenDatabaseBeforeTOTP(t *testing.T) {
	password := "correct horse battery staple"
	dbKey, err := randomDBKey()
	if err != nil {
		t.Fatalf("randomDBKey() error = %v", err)
	}
	passwordSalt, passwordHash, err := createPasswordVerifier(password)
	if err != nil {
		t.Fatalf("createPasswordVerifier() error = %v", err)
	}
	wrapped, err := wrapWithPassword(dbKey, password)
	if err != nil {
		t.Fatalf("wrapWithPassword() error = %v", err)
	}
	dataDir := t.TempDir()
	app := &App{
		dataDir:  dataDir,
		dbPath:   filepath.Join(dataDir, "foggy.db"),
		sessions: map[string]*session{},
		rate:     map[string]*rateBucket{},
		security: securityState{
			Version:             1,
			Initialized:         true,
			EncryptionProfile:   profileMaximumPrivacy,
			PasswordAuthEnabled: true,
			UserID:              "user_test",
			DisplayName:         "Test",
			PasswordSalt:        passwordSalt,
			PasswordHash:        passwordHash,
			TOTPSecret:          "JBSWY3DPEHPK3PXP",
			DBKeyWrapped:        wrapped,
			BackupCodes:         []backupCodeRecord{},
			Passkeys:            []passkeyCredential{},
		},
	}
	body := bytes.NewBufferString(`{"password":"correct horse battery staple","totpCode":"000000"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login-password", body)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	app.loginPassword(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("loginPassword() status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if app.isUnlocked() {
		t.Fatal("database was unlocked before TOTP was validated")
	}
}

func TestDevelopmentStatusAutoAuthUnlocksLocalhost(t *testing.T) {
	password := "correct horse battery staple"
	dbKey, err := randomDBKey()
	if err != nil {
		t.Fatalf("randomDBKey() error = %v", err)
	}
	passwordSalt, passwordHash, err := createPasswordVerifier(password)
	if err != nil {
		t.Fatalf("createPasswordVerifier() error = %v", err)
	}
	wrapped, err := wrapWithPassword(dbKey, password)
	if err != nil {
		t.Fatalf("wrapWithPassword() error = %v", err)
	}
	dataDir := t.TempDir()
	app := &App{
		dataDir:      dataDir,
		dbPath:       filepath.Join(dataDir, "foggy.db"),
		attachments:  filepath.Join(dataDir, "attachments"),
		origin:       "http://localhost:8080",
		devAutoAuth:  true,
		devPassword:  password,
		sessions:     map[string]*session{},
		rate:         map[string]*rateBucket{},
		requireHTTPS: true,
		security: securityState{
			Version:             1,
			Initialized:         true,
			EncryptionProfile:   profileMaximumPrivacy,
			PasswordAuthEnabled: true,
			UserID:              "user_test",
			DisplayName:         "Test",
			PasswordSalt:        passwordSalt,
			PasswordHash:        passwordHash,
			DBKeyWrapped:        wrapped,
			BackupCodes:         []backupCodeRecord{},
			Passkeys:            []passkeyCredential{},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()

	app.status(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status() status = %d, want %d", rec.Code, http.StatusOK)
	}
	var status statusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if !status.Authenticated {
		t.Fatal("status response was not authenticated")
	}
	if !status.Unlocked {
		t.Fatal("status response was not unlocked")
	}
	if status.CSRFToken == "" {
		t.Fatal("status response did not include a CSRF token")
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("development auto-auth did not set session cookies")
	}
}
