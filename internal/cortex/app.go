package cortex

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func NewApp(staticFS fs.FS) (*App, error) {
	dataDir := env("CORTEX_DATA_DIR", "./data")
	origin := env("CORTEX_ORIGIN", "http://localhost:8080")
	rpID := env("CORTEX_RP_ID", "localhost")
	addr := env("CORTEX_ADDR", ":8080")
	state, err := loadSecurityState(filepath.Join(dataDir, "security.json"))
	if err != nil {
		return nil, err
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Cortex",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, err
	}
	return &App{
		addr:          addr,
		dataDir:       dataDir,
		dbPath:        filepath.Join(dataDir, "cortex.db"),
		attachments:   filepath.Join(dataDir, "attachments"),
		serverKeyPath: filepath.Join(dataDir, "server.key"),
		origin:        origin,
		rpID:          rpID,
		requireHTTPS:  env("CORTEX_REQUIRE_HTTPS", "true") != "false",
		security:      state,
		sessions:      map[string]*session{},
		rate:          map[string]*rateBucket{},
		webAuthn:      wa,
		static:        spaHandler(staticFS),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:              a.addr,
		Handler:           a.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("cortex listening on %s", a.addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.health)
	mux.HandleFunc("/api/status", a.status)
	mux.HandleFunc("/api/setup", a.setup)
	mux.HandleFunc("/api/auth/login-password", a.loginPassword)
	mux.HandleFunc("/api/auth/recover", a.recoverWithBackupCode)
	mux.HandleFunc("/api/auth/logout", a.logout)
	mux.HandleFunc("/api/passkeys/register/options", a.passkeyRegisterOptions)
	mux.HandleFunc("/api/passkeys/register/finish", a.passkeyRegisterFinish)
	mux.HandleFunc("/api/passkeys/login/options", a.passkeyLoginOptions)
	mux.HandleFunc("/api/passkeys/login/finish", a.passkeyLoginFinish)
	mux.HandleFunc("/api/checkins", a.checkins)
	mux.HandleFunc("/api/symptoms", a.symptoms)
	mux.HandleFunc("/api/medications", a.medications)
	mux.HandleFunc("/api/attachments", a.attachmentsHandler)
	mux.HandleFunc("/api/settings", a.settings)
	mux.HandleFunc("/api/security/disable-password", a.disablePassword)
	mux.HandleFunc("/api/exports/clinician-summary", a.clinicianSummary)
	mux.Handle("/", a.static)
	return a.securityHeaders(a.httpsGuard(a.csrfGuard(mux)))
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self), payment=(), usb=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; media-src 'self' blob:; connect-src 'self'; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if isHTTPS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) httpsGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.requireHTTPS && !isHTTPS(r) && !isLocalHost(r.Host) {
			writeError(w, http.StatusUpgradeRequired, "HTTPS is required outside localhost")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if csrfExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := a.sessionFromRequest(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		token := r.Header.Get("X-CSRF-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(sess.CSRF)) != 1 {
			writeError(w, http.StatusForbidden, "CSRF token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func csrfExempt(path string) bool {
	switch path {
	case "/api/setup", "/api/auth/login-password", "/api/auth/recover", "/api/passkeys/login/options", "/api/passkeys/login/finish":
		return true
	default:
		return false
	}
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func isLocalHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) status(w http.ResponseWriter, r *http.Request) {
	sess, authenticated := a.sessionFromRequest(r)
	csrf := ""
	if authenticated {
		csrf = sess.CSRF
	}
	a.securityMu.RLock()
	state := a.security
	a.securityMu.RUnlock()
	resp := statusResponse{
		Initialized:         state.Initialized,
		Unlocked:            a.isUnlocked(),
		Authenticated:       authenticated,
		EncryptionProfile:   state.EncryptionProfile,
		PasswordAuthEnabled: state.PasswordAuthEnabled,
		PasskeysEnabled:     a.passkeyCount() > 0,
		CSRFToken:           csrf,
		DisplayName:         state.DisplayName,
		RequireHTTPS:        a.requireHTTPS,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) isUnlocked() bool {
	a.dbMu.RLock()
	defer a.dbMu.RUnlock()
	return a.db != nil
}

func (a *App) newSession(w http.ResponseWriter, userID string, stepUp bool) (*session, error) {
	id, err := randomSecretToken()
	if err != nil {
		return nil, err
	}
	csrf, err := randomSecretToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	sess := &session{ID: id, UserID: userID, CSRF: csrf, CreatedAt: now, LastSeenAt: now}
	if stepUp {
		sess.StepUpUntil = now.Add(15 * time.Minute)
	}
	a.sessionsMu.Lock()
	a.sessions[id] = sess
	a.sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "cortex_session",
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   strings.HasPrefix(a.origin, "https://"),
		MaxAge:   int((24 * time.Hour).Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "cortex_csrf",
		Value:    csrf,
		Path:     "/",
		HttpOnly: false,
		SameSite: http.SameSiteStrictMode,
		Secure:   strings.HasPrefix(a.origin, "https://"),
		MaxAge:   int((24 * time.Hour).Seconds()),
	})
	return sess, nil
}

func (a *App) clearSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("cortex_session"); err == nil {
		a.sessionsMu.Lock()
		delete(a.sessions, cookie.Value)
		a.sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "cortex_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.SetCookie(w, &http.Cookie{Name: "cortex_csrf", Path: "/", MaxAge: -1, SameSite: http.SameSiteStrictMode})
}

func (a *App) sessionFromRequest(r *http.Request) (*session, bool) {
	cookie, err := r.Cookie("cortex_session")
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	a.sessionsMu.Lock()
	defer a.sessionsMu.Unlock()
	sess, ok := a.sessions[cookie.Value]
	if !ok {
		return nil, false
	}
	if time.Since(sess.LastSeenAt) > 24*time.Hour {
		delete(a.sessions, cookie.Value)
		return nil, false
	}
	sess.LastSeenAt = time.Now().UTC()
	return sess, true
}

func (a *App) requireAuth(w http.ResponseWriter, r *http.Request) (*session, bool) {
	sess, ok := a.sessionFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Authentication required")
		return nil, false
	}
	return sess, true
}

func (a *App) allowAuthAttempt(r *http.Request) bool {
	ip := clientIP(r)
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	b := a.rate[ip]
	now := time.Now()
	if b == nil || now.After(b.ResetAt) {
		a.rate[ip] = &rateBucket{ResetAt: now.Add(10 * time.Minute)}
		return true
	}
	return b.Count < 8
}

func (a *App) recordAuthFailure(r *http.Request) {
	ip := clientIP(r)
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	b := a.rate[ip]
	if b == nil || time.Now().After(b.ResetAt) {
		b = &rateBucket{ResetAt: time.Now().Add(10 * time.Minute)}
		a.rate[ip] = b
	}
	b.Count++
	b.LastFailure = time.Now()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(staticFS, path); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func method(w http.ResponseWriter, r *http.Request, allowed string) bool {
	if r.Method != allowed {
		w.Header().Set("Allow", allowed)
		writeError(w, http.StatusMethodNotAllowed, fmt.Sprintf("%s required", allowed))
		return false
	}
	return true
}
