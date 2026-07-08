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
	a.securityMu.RLock()
	state := a.security
	a.securityMu.RUnlock()
	handle, err := base64.RawURLEncoding.DecodeString(state.UserHandle)
	if err != nil {
		return nil, err
	}
	creds := []webauthn.Credential{}
	for _, rec := range state.Passkeys {
		if rec.UserID != state.UserID {
			continue
		}
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(rec.CredentialJSON), &cred); err != nil {
			return nil, err
		}
		creds = append(creds, cred)
	}
	return &webUser{id: state.UserID, handle: handle, name: state.DisplayName, credentials: creds}, nil
}

func (a *App) passkeyCount() int {
	a.securityMu.RLock()
	defer a.securityMu.RUnlock()
	return len(a.security.Passkeys)
}

func (a *App) passkeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !a.isUnlocked() {
		writeError(w, http.StatusLocked, "Unlock Foggy with your password first")
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
	if !a.isUnlocked() {
		writeError(w, http.StatusLocked, "Unlock Foggy with your password first")
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
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	now := time.Now().UTC().Format(time.RFC3339)
	a.securityMu.Lock()
	replaced := false
	for i := range a.security.Passkeys {
		if a.security.Passkeys[i].ID == credentialID && a.security.Passkeys[i].UserID == user.id {
			a.security.Passkeys[i].CredentialJSON = string(b)
			a.security.Passkeys[i].CreatedAt = now
			replaced = true
			break
		}
	}
	if !replaced {
		a.security.Passkeys = append(a.security.Passkeys, passkeyCredential{
			ID:             credentialID,
			UserID:         user.id,
			CredentialJSON: string(b),
			CreatedAt:      now,
		})
	}
	state := a.security
	a.securityMu.Unlock()
	if err := saveSecurityState(a.securityPath(), state); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save passkey")
		return
	}
	a.audit("passkey_enrolled", map[string]any{"credential": credentialID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
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
	if !a.isUnlocked() {
		if err := a.unlockForConvenience(); err != nil {
			writeError(w, http.StatusLocked, requireUnlockedError().Error())
			return
		}
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
	b, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	a.securityMu.Lock()
	for i := range a.security.Passkeys {
		if a.security.Passkeys[i].ID == credentialID && a.security.Passkeys[i].UserID == user.id {
			a.security.Passkeys[i].CredentialJSON = string(b)
			a.security.Passkeys[i].LastUsedAt = time.Now().UTC().Format(time.RFC3339)
			state := a.security
			a.securityMu.Unlock()
			return saveSecurityState(a.securityPath(), state)
		}
	}
	a.securityMu.Unlock()
	return fmt.Errorf("unknown credential")
}
