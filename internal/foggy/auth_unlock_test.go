package foggy

import (
	"bytes"
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
