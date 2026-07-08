# Foggy

A secure, self-hosted symptom and wellness tracker for people with Multiple Sclerosis.

Foggy is designed for one person, local ownership, and low-friction logging. It runs as a container, stores health data in an encrypted SQLCipher SQLite database, and keeps attachments/audio memos encrypted beside the database in a mounted `data/` directory.

## What is implemented

- Single-user setup wizard with password, TOTP, exactly 4 one-time backup codes, and optional passkey enrollment.
- Password + TOTP login, passkey login, backup-code recovery, and guarded password-login disablement.
- Two encryption profiles:
  - Maximum privacy: the password is required after app/container restart to unwrap the SQLCipher database key.
  - Convenience passkey: a local `server.key` can unwrap the database after restart so passkey-only login works. This protects a copied database file, but not theft of the entire `data/` directory.
- Daily check-ins for MS burden, fatigue, energy, pain, mood, anxiety, brain fog, sleep, heat, mobility, bladder/bowel, and notes.
- Symptom events with category, specific symptom, body location/side, severity, duration, new/recurring state, function impact, triggers, heat exposure, infection signs, body temperature, treatment response, and relapse self-triage label.
- Medication/treatment events, encrypted attachment/audio memo upload, settings, and a clinician-summary JSON view.
- Mobile-first React UI with large tap targets, day/night mode, accent color, font size presets, high contrast, reduced motion, browser dictation where available, and local audio memo recording.

## Local Docker

```bash
docker compose up --build
```

Then open `http://localhost:8080`.

For a LAN/server deployment with passkeys, use HTTPS and set the origin/RP ID:

```bash
FOGGY_ORIGIN=https://foggy.example.com \
FOGGY_RP_ID=foggy.example.com \
FOGGY_DOMAIN=foggy.example.com \
FOGGY_ADMIN_EMAIL=you@example.com \
docker compose --profile tls up --build -d
```

## Data ownership

The important state lives in `./data` when using the default Compose file:

- `foggy.db`: SQLCipher-encrypted SQLite database.
- `security.json`: non-PHI auth/key metadata, including password verifier metadata, TOTP seed, passkey public credentials, recovery-code wrappers, and encryption profile state.
- `server.key`: only present for the convenience passkey profile.
- `attachments/`: encrypted uploaded files and audio memos.

Backups are intentionally left for a later phase, but copy/backup strategy should treat the entire `data/` directory as one unit.

## Development

For local UI iteration, Foggy can skip the login screen on localhost after setup. This is development-only and should not be enabled on a server:

```bash
FOGGY_DEV_AUTO_AUTH=true
FOGGY_DEV_AUTO_PASSWORD="your local development password"
```

Put those values in a local `.env` file when using Docker Compose. The app only honors development auto-auth when both `FOGGY_DEV_AUTO_AUTH=true` and the configured origin/request host are localhost-class addresses. Maximum-privacy databases still need `FOGGY_DEV_AUTO_PASSWORD` so the backend can unwrap the SQLCipher key after restart; convenience-passkey development data can use the local `server.key`.

Frontend:

```bash
cd web
npm install
npm run dev
```

Backend:

```bash
go test ./...
go run ./cmd/foggy
```

This checkout requires Go with CGO enabled because the backend uses the self-contained SQLCipher driver.

## Security notes

Foggy is built for personal self-hosted PHI protection. It is not a SaaS service and is not a formal HIPAA compliance package. If Foggy is ever hosted for other people, legal/compliance, incident response, BAA, operational monitoring, and breach-notification requirements need a separate design.

Passkeys require HTTPS or localhost-class secure contexts. Raw LAN/IP deployments should use password + TOTP until TLS is configured.
