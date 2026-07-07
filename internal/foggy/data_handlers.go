package foggy

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return v
}

func (a *App) checkins(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.listCheckins(w, r)
	case http.MethodPost:
		a.saveCheckin(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

func (a *App) listCheckins(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	rows, err := db.Query(`SELECT id, entry_date, overall_burden, fatigue, energy, pain, mood, anxiety, brain_fog,
		sleep_quality, heat_sensitivity, mobility, bladder_bowel, notes, created_at, updated_at
		FROM daily_checkins ORDER BY entry_date DESC LIMIT 30`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load check-ins")
		return
	}
	defer rows.Close()
	items := []dailyCheckIn{}
	for rows.Next() {
		var item dailyCheckIn
		if err := rows.Scan(&item.ID, &item.EntryDate, &item.OverallBurden, &item.Fatigue, &item.Energy, &item.Pain,
			&item.Mood, &item.Anxiety, &item.BrainFog, &item.SleepQuality, &item.HeatSensitivity, &item.Mobility,
			&item.BladderBowel, &item.Notes, &item.CreatedAt, &item.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not read check-ins")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) saveCheckin(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	var item dailyCheckIn
	if err := decodeJSON(r, &item); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid check-in")
		return
	}
	if item.EntryDate == "" {
		item.EntryDate = time.Now().Format("2006-01-02")
	}
	if item.ID == "" {
		item.ID, err = randomID("checkin")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create check-in")
			return
		}
	}
	item.OverallBurden = clampScore(item.OverallBurden)
	item.Fatigue = clampScore(item.Fatigue)
	item.Energy = clampScore(item.Energy)
	item.Pain = clampScore(item.Pain)
	item.Mood = clampScore(item.Mood)
	item.Anxiety = clampScore(item.Anxiety)
	item.BrainFog = clampScore(item.BrainFog)
	item.SleepQuality = clampScore(item.SleepQuality)
	item.HeatSensitivity = clampScore(item.HeatSensitivity)
	item.Mobility = clampScore(item.Mobility)
	item.BladderBowel = clampScore(item.BladderBowel)
	now := time.Now().UTC().Format(time.RFC3339)
	item.CreatedAt = now
	item.UpdatedAt = now
	_, err = db.Exec(`INSERT INTO daily_checkins(id, entry_date, overall_burden, fatigue, energy, pain, mood, anxiety, brain_fog,
		sleep_quality, heat_sensitivity, mobility, bladder_bowel, notes, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(entry_date) DO UPDATE SET
		overall_burden=excluded.overall_burden, fatigue=excluded.fatigue, energy=excluded.energy, pain=excluded.pain,
		mood=excluded.mood, anxiety=excluded.anxiety, brain_fog=excluded.brain_fog, sleep_quality=excluded.sleep_quality,
		heat_sensitivity=excluded.heat_sensitivity, mobility=excluded.mobility, bladder_bowel=excluded.bladder_bowel,
		notes=excluded.notes, updated_at=excluded.updated_at`,
		item.ID, item.EntryDate, item.OverallBurden, item.Fatigue, item.Energy, item.Pain, item.Mood, item.Anxiety,
		item.BrainFog, item.SleepQuality, item.HeatSensitivity, item.Mobility, item.BladderBowel, item.Notes, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save check-in")
		return
	}
	a.audit("daily_checkin_saved", map[string]any{"entry_date": item.EntryDate})
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) symptoms(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.listSymptoms(w, r)
	case http.MethodPost:
		a.saveSymptom(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

func (a *App) listSymptoms(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	rows, err := db.Query(`SELECT id, occurred_at, category, symptom, body_location, severity, duration, newness,
		function_impact, heat_exposure, triggers_json, infection_signs, body_temperature, treatment_response,
		relapse_flag, notes, created_at FROM symptom_events ORDER BY occurred_at DESC LIMIT 80`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load symptoms")
		return
	}
	defer rows.Close()
	items := []symptomEvent{}
	for rows.Next() {
		var item symptomEvent
		var triggers string
		if err := rows.Scan(&item.ID, &item.OccurredAt, &item.Category, &item.Symptom, &item.BodyLocation, &item.Severity,
			&item.Duration, &item.Newness, &item.FunctionImpact, &item.HeatExposure, &triggers, &item.InfectionSigns,
			&item.BodyTemperature, &item.TreatmentResponse, &item.RelapseFlag, &item.Notes, &item.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not read symptoms")
			return
		}
		_ = json.Unmarshal([]byte(triggers), &item.Triggers)
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) saveSymptom(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	var item symptomEvent
	if err := decodeJSON(r, &item); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid symptom")
		return
	}
	if strings.TrimSpace(item.Category) == "" || strings.TrimSpace(item.Symptom) == "" {
		writeError(w, http.StatusBadRequest, "Category and symptom are required")
		return
	}
	if item.ID == "" {
		item.ID, err = randomID("symptom")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create symptom")
			return
		}
	}
	if item.OccurredAt == "" {
		item.OccurredAt = time.Now().UTC().Format(time.RFC3339)
	}
	item.Severity = clampScore(item.Severity)
	if item.RelapseFlag == "" {
		item.RelapseFlag = "uncertain"
	}
	triggers, _ := json.Marshal(item.Triggers)
	item.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO symptom_events(id, occurred_at, category, symptom, body_location, severity, duration,
		newness, function_impact, heat_exposure, triggers_json, infection_signs, body_temperature, treatment_response,
		relapse_flag, notes, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.OccurredAt, item.Category, item.Symptom, item.BodyLocation, item.Severity, item.Duration,
		item.Newness, item.FunctionImpact, item.HeatExposure, string(triggers), item.InfectionSigns, item.BodyTemperature,
		item.TreatmentResponse, item.RelapseFlag, item.Notes, item.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save symptom")
		return
	}
	a.audit("symptom_saved", map[string]any{"category": item.Category, "relapse_flag": item.RelapseFlag})
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) medications(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.listMedications(w, r)
	case http.MethodPost:
		a.saveMedication(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

func (a *App) listMedications(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	rows, err := db.Query(`SELECT id, taken_at, name, dose, reason, effectiveness, side_effects, created_at
		FROM medication_events ORDER BY taken_at DESC LIMIT 80`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load medications")
		return
	}
	defer rows.Close()
	items := []medicationEvent{}
	for rows.Next() {
		var item medicationEvent
		if err := rows.Scan(&item.ID, &item.TakenAt, &item.Name, &item.Dose, &item.Reason, &item.Effectiveness, &item.SideEffects, &item.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Could not read medications")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *App) saveMedication(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	var item medicationEvent
	if err := decodeJSON(r, &item); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid medication")
		return
	}
	if strings.TrimSpace(item.Name) == "" {
		writeError(w, http.StatusBadRequest, "Medication name is required")
		return
	}
	if item.ID == "" {
		item.ID, err = randomID("med")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not create medication event")
			return
		}
	}
	if item.TakenAt == "" {
		item.TakenAt = time.Now().UTC().Format(time.RFC3339)
	}
	item.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO medication_events(id, taken_at, name, dose, reason, effectiveness, side_effects, created_at)
		VALUES(?,?,?,?,?,?,?,?)`, item.ID, item.TakenAt, item.Name, item.Dose, item.Reason, item.Effectiveness, item.SideEffects, item.CreatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save medication")
		return
	}
	a.audit("medication_saved", map[string]any{"name_recorded": true})
	writeJSON(w, http.StatusCreated, item)
}

func (a *App) attachmentsHandler(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAuth(w, r); !ok {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	dbKey, err := a.currentDBKey()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid attachment upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "File is required")
		return
	}
	defer file.Close()
	plain, err := io.ReadAll(io.LimitReader(file, 25<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Could not read attachment")
		return
	}
	nonce, ciphertext, err := encryptBlob(dbKey, "attachments", plain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not encrypt attachment")
		return
	}
	id, err := randomID("att")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not create attachment")
		return
	}
	storageName := id + ".bin"
	if err := os.MkdirAll(a.attachments, 0o700); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not prepare attachment storage")
		return
	}
	if err := os.WriteFile(filepath.Join(a.attachments, storageName), ciphertext, 0o600); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not store attachment")
		return
	}
	kind := r.FormValue("kind")
	if kind == "" {
		kind = "attachment"
	}
	relatedType := r.FormValue("relatedType")
	if relatedType == "" {
		relatedType = "note"
	}
	relatedID := r.FormValue("relatedID")
	if relatedID == "" {
		relatedID = "unlinked"
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(`INSERT INTO attachments(id, kind, related_type, related_id, filename, content_type, size_bytes,
		storage_path, nonce, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, kind, relatedType, relatedID, safeFilename(header.Filename),
		contentType, len(plain), storageName, encodeNonce(nonce), createdAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not record attachment")
		return
	}
	a.audit("attachment_saved", map[string]any{"kind": kind, "bytes": len(plain)})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "kind": kind, "filename": safeFilename(header.Filename), "contentType": contentType,
		"sizeBytes": len(plain), "createdAt": createdAt,
	})
}

func (a *App) settings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		a.getSettings(w, r)
	case http.MethodPost:
		a.saveSettings(w, r)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "GET or POST required")
	}
}

func (a *App) getSettings(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	var s userSettings
	var highContrast, reducedMotion int
	err = db.QueryRow(`SELECT theme, accent_color, font_scale, high_contrast, reduced_motion FROM user_settings WHERE id = 1`).
		Scan(&s.Theme, &s.AccentColor, &s.FontScale, &highContrast, &reducedMotion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load settings")
		return
	}
	s.HighContrast = highContrast == 1
	s.ReducedMotion = reducedMotion == 1
	writeJSON(w, http.StatusOK, s)
}

func (a *App) saveSettings(w http.ResponseWriter, r *http.Request) {
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	var s userSettings
	if err := decodeJSON(r, &s); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid settings")
		return
	}
	if s.Theme == "" {
		s.Theme = "system"
	}
	if s.AccentColor == "" {
		s.AccentColor = "#f97316"
	}
	if s.FontScale == "" {
		s.FontScale = "comfortable"
	}
	_, err = db.Exec(`UPDATE user_settings SET theme = ?, accent_color = ?, font_scale = ?, high_contrast = ?,
		reduced_motion = ?, updated_at = ? WHERE id = 1`, s.Theme, s.AccentColor, s.FontScale, boolInt(s.HighContrast),
		boolInt(s.ReducedMotion), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save settings")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (a *App) clinicianSummary(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAuth(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "GET required")
		return
	}
	db, err := a.dbConn()
	if err != nil {
		writeError(w, http.StatusLocked, "Unlock Foggy first")
		return
	}
	checkins := []dailyCheckIn{}
	symptoms := []symptomEvent{}
	meds := []medicationEvent{}
	rows, err := db.Query(`SELECT id, entry_date, overall_burden, fatigue, energy, pain, mood, anxiety, brain_fog,
		sleep_quality, heat_sensitivity, mobility, bladder_bowel, notes, created_at, updated_at
		FROM daily_checkins ORDER BY entry_date DESC LIMIT 14`)
	if err == nil {
		for rows.Next() {
			var item dailyCheckIn
			if rows.Scan(&item.ID, &item.EntryDate, &item.OverallBurden, &item.Fatigue, &item.Energy, &item.Pain,
				&item.Mood, &item.Anxiety, &item.BrainFog, &item.SleepQuality, &item.HeatSensitivity, &item.Mobility,
				&item.BladderBowel, &item.Notes, &item.CreatedAt, &item.UpdatedAt) == nil {
				checkins = append(checkins, item)
			}
		}
		rows.Close()
	}
	rows, err = db.Query(`SELECT id, occurred_at, category, symptom, body_location, severity, duration, newness,
		function_impact, heat_exposure, triggers_json, infection_signs, body_temperature, treatment_response,
		relapse_flag, notes, created_at FROM symptom_events ORDER BY occurred_at DESC LIMIT 40`)
	if err == nil {
		for rows.Next() {
			var item symptomEvent
			var triggers string
			if rows.Scan(&item.ID, &item.OccurredAt, &item.Category, &item.Symptom, &item.BodyLocation, &item.Severity,
				&item.Duration, &item.Newness, &item.FunctionImpact, &item.HeatExposure, &triggers, &item.InfectionSigns,
				&item.BodyTemperature, &item.TreatmentResponse, &item.RelapseFlag, &item.Notes, &item.CreatedAt) == nil {
				_ = json.Unmarshal([]byte(triggers), &item.Triggers)
				symptoms = append(symptoms, item)
			}
		}
		rows.Close()
	}
	rows, err = db.Query(`SELECT id, taken_at, name, dose, reason, effectiveness, side_effects, created_at
		FROM medication_events ORDER BY taken_at DESC LIMIT 30`)
	if err == nil {
		for rows.Next() {
			var item medicationEvent
			if rows.Scan(&item.ID, &item.TakenAt, &item.Name, &item.Dose, &item.Reason, &item.Effectiveness, &item.SideEffects, &item.CreatedAt) == nil {
				meds = append(meds, item)
			}
		}
		rows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"note":        "Self-tracked wellness data for clinician conversation; not diagnostic.",
		"checkins":    checkins,
		"symptoms":    symptoms,
		"medications": meds,
	})
}
