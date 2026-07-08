package foggy

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	profileMaximumPrivacy = "maximum_privacy"
	profileConvenience    = "convenience_passkey"
)

type securityState struct {
	Version             int                 `json:"version"`
	Initialized         bool                `json:"initialized"`
	EncryptionProfile   string              `json:"encryption_profile"`
	PasswordAuthEnabled bool                `json:"password_auth_enabled"`
	UserID              string              `json:"user_id"`
	UserHandle          string              `json:"user_handle"`
	DisplayName         string              `json:"display_name"`
	PasswordSalt        string              `json:"password_salt"`
	PasswordHash        string              `json:"password_hash"`
	TOTPSecret          string              `json:"totp_secret,omitempty"`
	DBKeyWrapped        wrappedKey          `json:"db_key_wrapped"`
	ServerKeyWrapped    *wrappedKey         `json:"server_key_wrapped,omitempty"`
	BackupCodes         []backupCodeRecord  `json:"backup_codes"`
	Passkeys            []passkeyCredential `json:"passkeys,omitempty"`
}

type backupCodeRecord struct {
	ID        string     `json:"id"`
	Salt      string     `json:"salt"`
	Hash      string     `json:"hash"`
	DBKeyWrap wrappedKey `json:"db_key_wrap"`
	UsedAt    *string    `json:"used_at,omitempty"`
	CreatedAt string     `json:"created_at"`
	LastFour  string     `json:"last_four"`
}

type passkeyCredential struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	CredentialJSON string `json:"credential_json"`
	CreatedAt      string `json:"created_at"`
	LastUsedAt     string `json:"last_used_at,omitempty"`
}

type session struct {
	ID              string
	UserID          string
	CSRF            string
	CreatedAt       time.Time
	LastSeenAt      time.Time
	StepUpUntil     time.Time
	WebAuthnSession *webauthn.SessionData
}

type rateBucket struct {
	Count       int
	ResetAt     time.Time
	LastFailure time.Time
}

type App struct {
	addr          string
	dataDir       string
	dbPath        string
	attachments   string
	serverKeyPath string
	origin        string
	rpID          string
	requireHTTPS  bool

	securityMu sync.RWMutex
	security   securityState

	dbMu  sync.RWMutex
	db    *sql.DB
	dbKey []byte

	sessionsMu sync.Mutex
	sessions   map[string]*session

	rateMu sync.Mutex
	rate   map[string]*rateBucket

	webAuthn *webauthn.WebAuthn
	static   http.Handler
}

type setupRequest struct {
	DisplayName       string `json:"displayName"`
	Password          string `json:"password"`
	EncryptionProfile string `json:"encryptionProfile"`
	AccentColor       string `json:"accentColor"`
	Theme             string `json:"theme"`
}

type setupResponse struct {
	Initialized       bool     `json:"initialized"`
	EncryptionProfile string   `json:"encryptionProfile"`
	TOTPSecret        string   `json:"totpSecret"`
	TOTPURL           string   `json:"totpUrl"`
	BackupCodes       []string `json:"backupCodes"`
	CSRFToken         string   `json:"csrfToken"`
}

type statusResponse struct {
	Initialized         bool   `json:"initialized"`
	Unlocked            bool   `json:"unlocked"`
	Authenticated       bool   `json:"authenticated"`
	EncryptionProfile   string `json:"encryptionProfile"`
	PasswordAuthEnabled bool   `json:"passwordAuthEnabled"`
	PasskeysEnabled     bool   `json:"passkeysEnabled"`
	CSRFToken           string `json:"csrfToken"`
	DisplayName         string `json:"displayName"`
	RequireHTTPS        bool   `json:"requireHttps"`
}

type loginPasswordRequest struct {
	Password string `json:"password"`
	TOTPCode string `json:"totpCode"`
}

type recoveryRequest struct {
	BackupCode  string `json:"backupCode"`
	NewPassword string `json:"newPassword"`
}

type disablePasswordRequest struct {
	TOTPCode           string `json:"totpCode"`
	BackupConfirmed    bool   `json:"backupConfirmed"`
	WarningAcknowledge bool   `json:"warningAcknowledge"`
}

type dailyCheckIn struct {
	ID              string `json:"id"`
	EntryDate       string `json:"entryDate"`
	OverallBurden   int    `json:"overallBurden"`
	Fatigue         int    `json:"fatigue"`
	Energy          int    `json:"energy"`
	Pain            int    `json:"pain"`
	Mood            int    `json:"mood"`
	Anxiety         int    `json:"anxiety"`
	BrainFog        int    `json:"brainFog"`
	SleepQuality    int    `json:"sleepQuality"`
	HeatSensitivity int    `json:"heatSensitivity"`
	Mobility        int    `json:"mobility"`
	BladderBowel    int    `json:"bladderBowel"`
	Notes           string `json:"notes"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type symptomEvent struct {
	ID                string   `json:"id"`
	OccurredAt        string   `json:"occurredAt"`
	Category          string   `json:"category"`
	Symptom           string   `json:"symptom"`
	BodyLocation      string   `json:"bodyLocation"`
	Severity          int      `json:"severity"`
	Duration          string   `json:"duration"`
	Newness           string   `json:"newness"`
	FunctionImpact    string   `json:"functionImpact"`
	HeatExposure      string   `json:"heatExposure"`
	Triggers          []string `json:"triggers"`
	InfectionSigns    string   `json:"infectionSigns"`
	BodyTemperature   string   `json:"bodyTemperature"`
	TreatmentResponse string   `json:"treatmentResponse"`
	RelapseFlag       string   `json:"relapseFlag"`
	Notes             string   `json:"notes"`
	CreatedAt         string   `json:"createdAt"`
}

type medicationEvent struct {
	ID            string `json:"id"`
	TakenAt       string `json:"takenAt"`
	Name          string `json:"name"`
	Dose          string `json:"dose"`
	Reason        string `json:"reason"`
	Effectiveness string `json:"effectiveness"`
	SideEffects   string `json:"sideEffects"`
	CreatedAt     string `json:"createdAt"`
}

type userSettings struct {
	Theme         string `json:"theme"`
	AccentColor   string `json:"accentColor"`
	FontScale     string `json:"fontScale"`
	HighContrast  bool   `json:"highContrast"`
	ReducedMotion bool   `json:"reducedMotion"`
}
