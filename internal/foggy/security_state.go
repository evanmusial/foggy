package foggy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func loadSecurityState(path string) (securityState, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return securityState{
			Version:             1,
			EncryptionProfile:   profileMaximumPrivacy,
			PasswordAuthEnabled: true,
			BackupCodes:         []backupCodeRecord{},
		}, nil
	}
	if err != nil {
		return securityState{}, err
	}
	var state securityState
	if err := json.Unmarshal(b, &state); err != nil {
		return securityState{}, err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.EncryptionProfile == "" {
		state.EncryptionProfile = profileMaximumPrivacy
	}
	if state.BackupCodes == nil {
		state.BackupCodes = []backupCodeRecord{}
	}
	if state.Passkeys == nil {
		state.Passkeys = []passkeyCredential{}
	}
	return state, nil
}

func saveSecurityState(path string, state securityState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *App) securityPath() string {
	return filepath.Join(a.dataDir, "security.json")
}

func (a *App) saveSecurity() error {
	a.securityMu.RLock()
	state := a.security
	a.securityMu.RUnlock()
	return saveSecurityState(a.securityPath(), state)
}

func createPasswordVerifier(password string) (saltB64 string, hashB64 string, err error) {
	salt, err := randomBytes(16)
	if err != nil {
		return "", "", err
	}
	hash := hashSecret(password, salt)
	return base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(hash), nil
}

func verifyPasswordState(state securityState, password string) bool {
	salt, err := mustDecodeB64(state.PasswordSalt)
	if err != nil {
		return false
	}
	hash, err := mustDecodeB64(state.PasswordHash)
	if err != nil {
		return false
	}
	return verifySecret(password, salt, hash)
}

func (a *App) rotatePassword(password string, dbKey []byte) error {
	saltB64, hashB64, err := createPasswordVerifier(password)
	if err != nil {
		return err
	}
	wrapped, err := wrapWithPassword(dbKey, password)
	if err != nil {
		return err
	}
	a.securityMu.Lock()
	a.security.PasswordSalt = saltB64
	a.security.PasswordHash = hashB64
	a.security.DBKeyWrapped = wrapped
	a.security.PasswordAuthEnabled = true
	a.securityMu.Unlock()
	return a.saveSecurity()
}

func (a *App) createBackupRecords(dbKey []byte) ([]backupCodeRecord, []string, error) {
	records := make([]backupCodeRecord, 0, 4)
	codes := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		code, err := newBackupCode()
		if err != nil {
			return nil, nil, err
		}
		id, err := randomID("recovery")
		if err != nil {
			return nil, nil, err
		}
		salt, err := randomBytes(16)
		if err != nil {
			return nil, nil, err
		}
		hash := hashSecret(normalizeCode(code), salt)
		wrapped, err := wrapWithPassword(dbKey, normalizeCode(code))
		if err != nil {
			return nil, nil, err
		}
		records = append(records, backupCodeRecord{
			ID:        id,
			Salt:      base64.RawURLEncoding.EncodeToString(salt),
			Hash:      base64.RawURLEncoding.EncodeToString(hash),
			DBKeyWrap: wrapped,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			LastFour:  code[len(code)-4:],
		})
		codes = append(codes, code)
	}
	return records, codes, nil
}

func (a *App) consumeBackupCode(code string) ([]byte, error) {
	code = normalizeCode(code)
	a.securityMu.Lock()
	defer a.securityMu.Unlock()
	for i := range a.security.BackupCodes {
		rec := &a.security.BackupCodes[i]
		if rec.UsedAt != nil {
			continue
		}
		salt, err := mustDecodeB64(rec.Salt)
		if err != nil {
			continue
		}
		hash, err := mustDecodeB64(rec.Hash)
		if err != nil {
			continue
		}
		if !verifySecret(code, salt, hash) {
			continue
		}
		dbKey, err := unwrapWithPassword(rec.DBKeyWrap, code)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC().Format(time.RFC3339)
		rec.UsedAt = &now
		if err := saveSecurityState(a.securityPath(), a.security); err != nil {
			return nil, err
		}
		return dbKey, nil
	}
	return nil, fmt.Errorf("invalid recovery code")
}

func (a *App) ensureServerKey(dbKey []byte) error {
	if a.security.EncryptionProfile != profileConvenience {
		return nil
	}
	if err := os.MkdirAll(a.dataDir, 0o700); err != nil {
		return err
	}
	serverKey, err := randomBytes(32)
	if err != nil {
		return err
	}
	if err := os.WriteFile(a.serverKeyPath, serverKey, 0o600); err != nil {
		return err
	}
	wrapped, err := wrapWithRawServerKey(dbKey, serverKey)
	if err != nil {
		return err
	}
	a.security.ServerKeyWrapped = &wrapped
	return nil
}

func (a *App) unlockWithServerKey() ([]byte, error) {
	a.securityMu.RLock()
	wrapped := a.security.ServerKeyWrapped
	a.securityMu.RUnlock()
	if wrapped == nil {
		return nil, fmt.Errorf("server key wrapping is unavailable")
	}
	serverKey, err := os.ReadFile(a.serverKeyPath)
	if err != nil {
		return nil, err
	}
	return unwrapWithRawServerKey(*wrapped, serverKey)
}
