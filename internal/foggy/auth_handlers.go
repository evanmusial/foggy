package foggy

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
)

func (a *App) setup(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	a.securityMu.RLock()
	already := a.security.Initialized
	a.securityMu.RUnlock()
	if already {
		writeError(w, http.StatusConflict, "Foggy is already set up")
		return
	}
	var req setupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid setup request")
		return
	}
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.DisplayName == "" {
		req.DisplayName = "Foggy user"
	}
	if len(req.Password) < 12 {
		writeError(w, http.StatusBadRequest, "Use a password with at least 12 characters")
		return
	}
	if req.EncryptionProfile == "" {
		req.EncryptionProfile = profileMaximumPrivacy
	}
	if req.EncryptionProfile != profileMaximumPrivacy && req.EncryptionProfile != profileConvenience {
		writeError(w, http.StatusBadRequest, "Unknown encryption profile")
		return
	}
	dbKey, err := randomDBKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create key material")
		return
	}
	userID, err := randomID("user")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create user")
		return
	}
	handle, err := randomBytes(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create passkey handle")
		return
	}
	passwordSalt, passwordHash, err := createPasswordVerifier(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not protect password")
		return
	}
	wrapped, err := wrapWithPassword(dbKey, req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not protect database key")
		return
	}
	backupRecords, backupCodes, err := a.createBackupRecords(dbKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create recovery codes")
		return
	}
	state := securityState{
		Version:             1,
		Initialized:         true,
		EncryptionProfile:   req.EncryptionProfile,
		PasswordAuthEnabled: true,
		UserID:              userID,
		UserHandle:          base64.RawURLEncoding.EncodeToString(handle),
		DisplayName:         req.DisplayName,
		PasswordSalt:        passwordSalt,
		PasswordHash:        passwordHash,
		DBKeyWrapped:        wrapped,
		BackupCodes:         backupRecords,
	}
	a.securityMu.Lock()
	a.security = state
	if err := a.ensureServerKey(dbKey); err != nil {
		a.securityMu.Unlock()
		writeError(w, http.StatusInternalServerError, "Could not create convenience key")
		return
	}
	state = a.security
	a.securityMu.Unlock()
	if err := saveSecurityState(a.securityPath(), state); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save security metadata")
		return
	}
	if err := a.openEncryptedDB(dbKey); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not initialize encrypted database")
		return
	}
	if err := a.initializeUserSettings(req); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save settings")
		return
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Foggy",
		AccountName: req.DisplayName,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create TOTP secret")
		return
	}
	if err := a.saveTOTPSecret(key.Secret()); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save TOTP secret")
		return
	}
	sess, err := a.newSession(w, userID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	a.audit("setup_completed", map[string]any{"profile": req.EncryptionProfile})
	writeJSON(w, http.StatusCreated, setupResponse{
		Initialized:       true,
		EncryptionProfile: req.EncryptionProfile,
		TOTPSecret:        key.Secret(),
		TOTPURL:           key.URL(),
		BackupCodes:       backupCodes,
		CSRFToken:         sess.CSRF,
	})
}

func (a *App) initializeUserSettings(req setupRequest) error {
	db, err := a.dbConn()
	if err != nil {
		return err
	}
	accent := strings.TrimSpace(req.AccentColor)
	if accent == "" {
		accent = "#f97316"
	}
	theme := strings.TrimSpace(req.Theme)
	if theme == "" {
		theme = "system"
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO user_settings(id, theme, accent_color, font_scale, high_contrast, reduced_motion, updated_at)
		VALUES(1, ?, ?, 'comfortable', 0, 0, ?)`, theme, accent, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (a *App) saveTOTPSecret(secret string) error {
	db, err := a.dbConn()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO auth_totp(id, secret, enabled, created_at) VALUES(1, ?, 1, ?)`,
		secret, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (a *App) verifyTOTP(code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	db, err := a.dbConn()
	if err != nil {
		return false
	}
	var secret string
	var enabled int
	if err := db.QueryRow(`SELECT secret, enabled FROM auth_totp WHERE id = 1`).Scan(&secret, &enabled); err != nil {
		return false
	}
	if enabled != 1 {
		return false
	}
	return totp.Validate(code, secret)
}

func (a *App) loginPassword(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !a.allowAuthAttempt(r) {
		writeError(w, http.StatusTooManyRequests, "Too many login attempts. Try again later.")
		return
	}
	var req loginPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid login request")
		return
	}
	if !a.isUnlocked() {
		if err := a.unlockWithPassword(req.Password); err != nil {
			a.recordAuthFailure(r)
			writeError(w, http.StatusUnauthorized, "Invalid login")
			return
		}
	} else {
		a.securityMu.RLock()
		state := a.security
		a.securityMu.RUnlock()
		if state.PasswordAuthEnabled && !verifyPasswordState(state, req.Password) {
			a.recordAuthFailure(r)
			writeError(w, http.StatusUnauthorized, "Invalid login")
			return
		}
	}
	if !a.verifyTOTP(req.TOTPCode) {
		a.recordAuthFailure(r)
		writeError(w, http.StatusUnauthorized, "Invalid login")
		return
	}
	a.securityMu.RLock()
	userID := a.security.UserID
	a.securityMu.RUnlock()
	sess, err := a.newSession(w, userID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	a.audit("login_password", map[string]any{"method": "password_totp"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrfToken": sess.CSRF})
}

func (a *App) recoverWithBackupCode(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !a.allowAuthAttempt(r) {
		writeError(w, http.StatusTooManyRequests, "Too many recovery attempts. Try again later.")
		return
	}
	var req recoveryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid recovery request")
		return
	}
	if strings.TrimSpace(req.NewPassword) != "" && len(req.NewPassword) < 12 {
		writeError(w, http.StatusBadRequest, "Use a password with at least 12 characters")
		return
	}
	dbKey, err := a.consumeBackupCode(req.BackupCode)
	if err != nil {
		a.recordAuthFailure(r)
		writeError(w, http.StatusUnauthorized, "Invalid recovery code")
		return
	}
	if err := a.openEncryptedDB(dbKey); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not unlock database")
		return
	}
	if strings.TrimSpace(req.NewPassword) != "" {
		if err := a.rotatePassword(req.NewPassword, dbKey); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not rotate password")
			return
		}
	}
	a.securityMu.RLock()
	userID := a.security.UserID
	a.securityMu.RUnlock()
	sess, err := a.newSession(w, userID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	a.audit("recovery_code_used", map[string]any{"password_rotated": req.NewPassword != ""})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrfToken": sess.CSRF})
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	a.clearSession(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) disablePassword(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	sess, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	if time.Now().UTC().After(sess.StepUpUntil) {
		writeError(w, http.StatusForbidden, "Recent password and MFA verification required")
		return
	}
	var req disablePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request")
		return
	}
	if !req.BackupConfirmed || !req.WarningAcknowledge {
		writeError(w, http.StatusBadRequest, "Confirm backup codes and warning before disabling password login")
		return
	}
	if a.passkeyCount() == 0 {
		writeError(w, http.StatusBadRequest, "Enroll at least one passkey before disabling password login")
		return
	}
	if !a.verifyTOTP(req.TOTPCode) {
		writeError(w, http.StatusUnauthorized, "Invalid verification code")
		return
	}
	a.securityMu.Lock()
	a.security.PasswordAuthEnabled = false
	a.securityMu.Unlock()
	if err := a.saveSecurity(); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update security settings")
		return
	}
	a.audit("password_auth_disabled", map[string]any{"profile": a.security.EncryptionProfile})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) authError(w http.ResponseWriter, r *http.Request) {
	a.recordAuthFailure(r)
	writeError(w, http.StatusUnauthorized, "Invalid login")
}

func requireUnlockedError() error {
	return fmt.Errorf("database is locked; password unlock is required in maximum privacy mode")
}
