package foggy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

type webUser struct {
	id          string
	handle      []byte
	name        string
	credentials []webauthn.Credential
}

func (u *webUser) WebAuthnID() []byte          { return u.handle }
func (u *webUser) WebAuthnName() string        { return u.name }
func (u *webUser) WebAuthnDisplayName() string { return u.name }
func (u *webUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func (a *App) loadWebUser() (*webUser, error) {
	db, err := a.dbConn()
	if err != nil {
		return nil, err
	}
	a.securityMu.RLock()
	state := a.security
	a.securityMu.RUnlock()
	handle, err := base64.RawURLEncoding.DecodeString(state.UserHandle)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT credential_json FROM webauthn_credentials WHERE user_id = ?`, state.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	creds := []webauthn.Credential{}
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var cred webauthn.Credential
		if err := json.Unmarshal(b, &cred); err != nil {
			return nil, err
		}
		creds = append(creds, cred)
	}
	return &webUser{id: state.UserID, handle: handle, name: state.DisplayName, credentials: creds}, rows.Err()
}

func (a *App) passkeyCount() int {
	db, err := a.dbConn()
	if err != nil {
		return 0
	}
	var count int
	_ = db.QueryRow(`SELECT count(*) FROM webauthn_credentials`).Scan(&count)
	return count
}

func (a *App) passkeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	sess, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	user, err := a.loadWebUser()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy with your password first")
		return
	}
	creation, webSession, err := a.webAuthn.BeginMediatedRegistration(
		user,
		protocol.MediationDefault,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			RequireResidentKey: protocol.ResidentKeyRequired(),
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			UserVerification:   protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(webauthn.Credentials(user.WebAuthnCredentials()).CredentialDescriptors()),
		webauthn.WithExtensions(protocol.AuthenticationExtensions{"credProps": true}),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start passkey enrollment")
		return
	}
	a.sessionsMu.Lock()
	sess.WebAuthnSession = webSession
	a.sessionsMu.Unlock()
	writeJSON(w, http.StatusOK, creation)
}

func (a *App) passkeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	sess, ok := a.requireAuth(w, r)
	if !ok {
		return
	}
	user, err := a.loadWebUser()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy with your password first")
		return
	}
	a.sessionsMu.Lock()
	webSession := sess.WebAuthnSession
	sess.WebAuthnSession = nil
	a.sessionsMu.Unlock()
	if webSession == nil {
		writeError(w, http.StatusBadRequest, "Passkey enrollment was not started")
		return
	}
	credential, err := a.webAuthn.FinishRegistration(user, *webSession, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Passkey enrollment failed")
		return
	}
	b, err := json.Marshal(credential)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save passkey")
		return
	}
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy with your password first")
		return
	}
	_, err = db.Exec(`INSERT OR REPLACE INTO webauthn_credentials(id, user_id, credential_json, created_at) VALUES(?,?,?,?)`,
		base64.RawURLEncoding.EncodeToString(credential.ID), user.id, b, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save passkey")
		return
	}
	a.audit("passkey_enrolled", map[string]any{"credential": base64.RawURLEncoding.EncodeToString(credential.ID)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !a.isUnlocked() {
		if err := a.unlockForConvenience(); err != nil {
			writeError(w, http.StatusLocked, requireUnlockedError().Error())
			return
		}
	}
	assertion, webSession, err := a.webAuthn.BeginDiscoverableMediatedLogin(
		protocol.MediationDefault,
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not start passkey login")
		return
	}
	tempSess, err := a.newSession(w, "pending-passkey", false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create passkey session")
		return
	}
	a.sessionsMu.Lock()
	tempSess.WebAuthnSession = webSession
	a.sessionsMu.Unlock()
	writeJSON(w, http.StatusOK, assertion)
}

func (a *App) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !a.isUnlocked() {
		if err := a.unlockForConvenience(); err != nil {
			writeError(w, http.StatusLocked, requireUnlockedError().Error())
			return
		}
	}
	sess, ok := a.sessionFromRequest(r)
	if !ok || sess.WebAuthnSession == nil {
		writeError(w, http.StatusBadRequest, "Passkey login was not started")
		return
	}
	a.sessionsMu.Lock()
	webSession := sess.WebAuthnSession
	sess.WebAuthnSession = nil
	a.sessionsMu.Unlock()
	user, credential, err := a.webAuthn.FinishPasskeyLogin(a.discoverableUser, *webSession, r)
	if err != nil {
		a.recordAuthFailure(r)
		writeError(w, http.StatusUnauthorized, "Invalid login")
		return
	}
	webUser, ok := user.(*webUser)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Could not load passkey user")
		return
	}
	if err := a.updateCredential(webUser, credential); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not update passkey")
		return
	}
	newSess, err := a.newSession(w, webUser.id, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create session")
		return
	}
	a.audit("login_passkey", map[string]any{"method": "passkey"})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "csrfToken": newSess.CSRF})
}

func (a *App) discoverableUser(rawID []byte, userHandle []byte) (webauthn.User, error) {
	user, err := a.loadWebUser()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(user.handle, userHandle) {
		return nil, fmt.Errorf("unknown user handle")
	}
	for _, cred := range user.credentials {
		if bytes.Equal(cred.ID, rawID) {
			return user, nil
		}
	}
	return nil, fmt.Errorf("unknown credential")
}

func (a *App) updateCredential(user *webUser, credential *webauthn.Credential) error {
	db, err := a.dbConn()
	if err != nil {
		return err
	}
	b, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE webauthn_credentials SET credential_json = ?, last_used_at = ? WHERE id = ? AND user_id = ?`,
		b, time.Now().UTC().Format(time.RFC3339), base64.RawURLEncoding.EncodeToString(credential.ID), user.id)
	return err
}
