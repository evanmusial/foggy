import { useEffect, useRef, useState } from 'react';
import { api } from './api';
import { createPasskey, getPasskey } from './passkeys';
import type { DailyCheckIn, MedicationEvent, Settings, Status, SymptomEvent } from './types';

type View = 'home' | 'checkin' | 'symptom' | 'voice' | 'clinician' | 'settings';

const today = () => new Date().toISOString().slice(0, 10);
const nowLocal = () => new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16);
const toLocalInput = (iso: string) => {
  const date = new Date(iso);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
};

const defaultSettings: Settings = {
  theme: 'system',
  accentColor: '#f97316',
  fontScale: 'comfortable',
  highContrast: false,
  reducedMotion: false
};

const blankCheckin = (): DailyCheckIn => ({
  entryDate: today(),
  overallBurden: 4,
  fatigue: 5,
  energy: 5,
  pain: 3,
  mood: 6,
  anxiety: 3,
  brainFog: 4,
  sleepQuality: 5,
  heatSensitivity: 3,
  mobility: 5,
  bladderBowel: 3,
  notes: ''
});

const blankSymptom = (): SymptomEvent => ({
  occurredAt: new Date().toISOString(),
  category: 'fatigue',
  symptom: '',
  bodyLocation: '',
  severity: 5,
  duration: '',
  newness: 'recurring',
  functionImpact: '',
  heatExposure: '',
  triggers: [],
  infectionSigns: '',
  bodyTemperature: '',
  treatmentResponse: '',
  relapseFlag: 'uncertain',
  notes: ''
});

const blankMedication = (): MedicationEvent => ({
  takenAt: new Date().toISOString(),
  name: '',
  dose: '',
  reason: '',
  effectiveness: '',
  sideEffects: ''
});

export function App() {
  const [status, setStatus] = useState<Status | null>(null);
  const [settings, setSettings] = useState<Settings>(defaultSettings);
  const [view, setView] = useState<View>('home');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  const refresh = async () => {
    const next = await api.status();
    setStatus(next);
    if (next.authenticated) {
      try {
        setSettings(await api.settings());
      } catch {
        setSettings(defaultSettings);
      }
    }
  };

  useEffect(() => {
    refresh().catch((err) => setMessage(err.message));
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = settings.theme;
    document.documentElement.dataset.fontScale = settings.fontScale;
    document.documentElement.dataset.contrast = settings.highContrast ? 'high' : 'normal';
    document.documentElement.dataset.motion = settings.reducedMotion ? 'reduced' : 'normal';
    document.documentElement.style.setProperty('--accent', settings.accentColor);
  }, [settings]);

  const run = async (work: () => Promise<void>, success?: string) => {
    setBusy(true);
    setMessage('');
    try {
      await work();
      if (success) setMessage(success);
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  };

  if (!status) return <main className="center-shell"><p>Loading Cortex...</p></main>;

  if (!status.initialized) {
    return <SetupScreen busy={busy} message={message} onRun={run} onDone={refresh} />;
  }

  if (!status.authenticated) {
    return <LoginScreen busy={busy} status={status} message={message} onRun={run} onDone={refresh} />;
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">Cortex</p>
          <h1>{viewTitle(view)}</h1>
        </div>
        <button className="small-button" onClick={() => run(async () => { await api.logout(); await refresh(); }, 'Locked.')}>Lock</button>
      </header>
      {message && <div className="notice" role="status">{message}</div>}
      <main>
        {view === 'home' && <HomeScreen setView={setView} />}
        {view === 'checkin' && <CheckInScreen run={run} />}
        {view === 'symptom' && <SymptomScreen run={run} />}
        {view === 'voice' && <VoiceScreen run={run} />}
        {view === 'clinician' && <ClinicianScreen run={run} />}
        {view === 'settings' && <SettingsScreen status={status} settings={settings} setSettings={setSettings} run={run} refresh={refresh} />}
      </main>
      <nav className="bottom-nav" aria-label="Primary">
        {(['home', 'checkin', 'symptom', 'voice', 'clinician', 'settings'] as View[]).map((item) => (
          <button key={item} className={view === item ? 'active' : ''} onClick={() => setView(item)}>
            {navLabel(item)}
          </button>
        ))}
      </nav>
      {busy && <div className="busy" aria-live="polite">Working...</div>}
    </div>
  );
}

function viewTitle(view: View) {
  return {
    home: 'Today',
    checkin: 'Daily check-in',
    symptom: 'Log symptom',
    voice: 'Voice note',
    clinician: 'Summary',
    settings: 'Settings'
  }[view];
}

function navLabel(view: View) {
  return {
    home: 'Today',
    checkin: 'Check',
    symptom: 'Log',
    voice: 'Voice',
    clinician: 'Share',
    settings: 'Tune'
  }[view];
}

function SetupScreen({ busy, message, onRun, onDone }: { busy: boolean; message: string; onRun: (w: () => Promise<void>, s?: string) => void; onDone: () => Promise<void> }) {
  const [displayName, setDisplayName] = useState('Me');
  const [password, setPassword] = useState('');
  const [profile, setProfile] = useState('maximum_privacy');
  const [setupResult, setSetupResult] = useState<{ backupCodes: string[]; totpSecret: string; totpUrl: string } | null>(null);
  return (
    <main className="center-shell">
      <section className="panel setup-panel">
        <p className="eyebrow">Private first</p>
        <h1>Cortex</h1>
        <p className="lead">A self-hosted MS wellness log for fast, protected daily tracking.</p>
        {!setupResult ? (
          <form onSubmit={(e) => {
            e.preventDefault();
            onRun(async () => {
              const result = await api.setup({ displayName, password, encryptionProfile: profile, accentColor: '#f97316', theme: 'system' });
              setSetupResult(result);
            });
          }}>
            <label>Name<input value={displayName} onChange={(e) => setDisplayName(e.target.value)} /></label>
            <label>Password<input type="password" value={password} minLength={12} onChange={(e) => setPassword(e.target.value)} /></label>
            <fieldset>
              <legend>Encryption profile</legend>
              <label className="choice"><input type="radio" checked={profile === 'maximum_privacy'} onChange={() => setProfile('maximum_privacy')} /> Maximum privacy</label>
              <label className="choice"><input type="radio" checked={profile === 'convenience_passkey'} onChange={() => setProfile('convenience_passkey')} /> Convenience passkey</label>
            </fieldset>
            <button className="primary" disabled={busy}>Create protected log</button>
          </form>
        ) : (
          <div className="result-block">
            <h2>MFA is ready</h2>
            <p>Scan or enter this TOTP secret in your authenticator app.</p>
            <code>{setupResult.totpSecret}</code>
            <p className="muted">Backup codes. They are shown once and each works once.</p>
            <div className="code-grid">{setupResult.backupCodes.map((code) => <code key={code}>{code}</code>)}</div>
            <button className="primary" onClick={() => onDone()}>Enter Cortex</button>
          </div>
        )}
        {message && <p className="error">{message}</p>}
      </section>
    </main>
  );
}

function LoginScreen({ busy, status, message, onRun, onDone }: { busy: boolean; status: Status; message: string; onRun: (w: () => Promise<void>, s?: string) => void; onDone: () => Promise<void>; }) {
  const [password, setPassword] = useState('');
  const [totp, setTotp] = useState('');
  const [recovery, setRecovery] = useState(false);
  const [backupCode, setBackupCode] = useState('');
  const [newPassword, setNewPassword] = useState('');
  return (
    <main className="center-shell">
      <section className="panel login-panel">
        <p className="eyebrow">Welcome back</p>
        <h1>Cortex</h1>
        {!recovery ? (
          <form onSubmit={(e) => {
            e.preventDefault();
            onRun(async () => { await api.loginPassword(password, totp); await onDone(); });
          }}>
            {status.passwordAuthEnabled && <label>Password<input type="password" value={password} onChange={(e) => setPassword(e.target.value)} /></label>}
            <label>Authenticator code<input inputMode="numeric" value={totp} onChange={(e) => setTotp(e.target.value)} /></label>
            <button className="primary" disabled={busy}>Unlock</button>
            {status.passkeysEnabled && (
              <button type="button" className="secondary" onClick={() => onRun(async () => {
                const options = await api.passkeyLoginOptions();
                const credential = await getPasskey(options);
                await api.passkeyLoginFinish(credential);
                await onDone();
              })}>Use passkey</button>
            )}
            <button type="button" className="link-button" onClick={() => setRecovery(true)}>Use backup code</button>
          </form>
        ) : (
          <form onSubmit={(e) => {
            e.preventDefault();
            onRun(async () => { await api.recover(backupCode, newPassword); await onDone(); });
          }}>
            <label>Backup code<input value={backupCode} onChange={(e) => setBackupCode(e.target.value)} /></label>
            <label>New password<input type="password" value={newPassword} minLength={12} onChange={(e) => setNewPassword(e.target.value)} /></label>
            <button className="primary" disabled={busy}>Recover access</button>
            <button type="button" className="link-button" onClick={() => setRecovery(false)}>Back to login</button>
          </form>
        )}
        {message && <p className="error">{message}</p>}
      </section>
    </main>
  );
}

function HomeScreen({ setView }: { setView: (v: View) => void }) {
  return (
    <section className="home-grid">
      <button className="action-tile largest" onClick={() => setView('symptom')}>
        <span>Log now</span><small>Symptom, trigger, impact</small>
      </button>
      <button className="action-tile" onClick={() => setView('checkin')}>
        <span>Daily check-in</span><small>Fast whole-day snapshot</small>
      </button>
      <button className="action-tile" onClick={() => setView('voice')}>
        <span>Voice note</span><small>Dictate or save audio</small>
      </button>
      <button className="action-tile quiet" onClick={() => setView('clinician')}>
        <span>Visit summary</span><small>Trends and recent events</small>
      </button>
    </section>
  );
}

function Score({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <label className="score-row">
      <span>{label}</span>
      <strong>{value}</strong>
      <input type="range" min={0} max={10} value={value} onChange={(e) => onChange(Number(e.target.value))} />
    </label>
  );
}

function CheckInScreen({ run }: { run: (w: () => Promise<void>, s?: string) => void }) {
  const [item, setItem] = useState(blankCheckin);
  const set = (key: keyof DailyCheckIn, value: string | number) => setItem((prev) => ({ ...prev, [key]: value }));
  return (
    <form className="flow" onSubmit={(e) => {
      e.preventDefault();
      run(async () => { await api.saveCheckin(item); setItem(blankCheckin()); }, 'Daily check-in saved.');
    }}>
      <label>Date<input type="date" value={item.entryDate} onChange={(e) => set('entryDate', e.target.value)} /></label>
      <div className="score-grid">
        <Score label="Overall MS burden" value={item.overallBurden} onChange={(v) => set('overallBurden', v)} />
        <Score label="Fatigue" value={item.fatigue} onChange={(v) => set('fatigue', v)} />
        <Score label="Energy" value={item.energy} onChange={(v) => set('energy', v)} />
        <Score label="Pain" value={item.pain} onChange={(v) => set('pain', v)} />
        <Score label="Mood" value={item.mood} onChange={(v) => set('mood', v)} />
        <Score label="Anxiety" value={item.anxiety} onChange={(v) => set('anxiety', v)} />
        <Score label="Brain fog" value={item.brainFog} onChange={(v) => set('brainFog', v)} />
        <Score label="Sleep quality" value={item.sleepQuality} onChange={(v) => set('sleepQuality', v)} />
        <Score label="Heat sensitivity" value={item.heatSensitivity} onChange={(v) => set('heatSensitivity', v)} />
        <Score label="Mobility" value={item.mobility} onChange={(v) => set('mobility', v)} />
        <Score label="Bladder/bowel" value={item.bladderBowel} onChange={(v) => set('bladderBowel', v)} />
      </div>
      <DictationArea label="Notes" value={item.notes} onChange={(v) => set('notes', v)} />
      <button className="primary">Save check-in</button>
    </form>
  );
}

function SymptomScreen({ run }: { run: (w: () => Promise<void>, s?: string) => void }) {
  const [item, setItem] = useState(blankSymptom);
  const [triggerText, setTriggerText] = useState('');
  const set = (key: keyof SymptomEvent, value: string | number | string[]) => setItem((prev) => ({ ...prev, [key]: value }));
  return (
    <form className="flow" onSubmit={(e) => {
      e.preventDefault();
      run(async () => {
        await api.saveSymptom({ ...item, occurredAt: new Date(item.occurredAt).toISOString(), triggers: triggerText.split(',').map((t) => t.trim()).filter(Boolean) });
        setItem(blankSymptom());
        setTriggerText('');
      }, 'Symptom saved.');
    }}>
      <label>When<input type="datetime-local" value={toLocalInput(item.occurredAt)} onChange={(e) => set('occurredAt', new Date(e.target.value).toISOString())} /></label>
      <label>Category<select value={item.category} onChange={(e) => set('category', e.target.value)}>
        {['fatigue', 'pain', 'vision', 'mobility', 'cognition', 'mood', 'bladder/bowel', 'spasticity', 'tremor', 'numbness', 'weakness', 'sleep', 'temperature'].map((x) => <option key={x}>{x}</option>)}
      </select></label>
      <DictationArea label="What happened?" value={item.symptom} onChange={(v) => set('symptom', v)} />
      <label>Body location or side<input value={item.bodyLocation} onChange={(e) => set('bodyLocation', e.target.value)} /></label>
      <Score label="Severity" value={item.severity} onChange={(v) => set('severity', v)} />
      <label>Duration<input value={item.duration} onChange={(e) => set('duration', e.target.value)} placeholder="minutes, hours, all day, ongoing" /></label>
      <label>New or recurring<select value={item.newness} onChange={(e) => set('newness', e.target.value)}>
        <option value="new">new</option><option value="recurring">recurring</option><option value="worse-baseline">worse baseline</option><option value="returned">returned</option>
      </select></label>
      <DictationArea label="Function impact" value={item.functionImpact} onChange={(v) => set('functionImpact', v)} />
      <label>Possible triggers<input value={triggerText} onChange={(e) => setTriggerText(e.target.value)} placeholder="heat, poor sleep, stress" /></label>
      <label>Heat or temperature exposure<input value={item.heatExposure} onChange={(e) => set('heatExposure', e.target.value)} /></label>
      <label>Infection signs<input value={item.infectionSigns} onChange={(e) => set('infectionSigns', e.target.value)} placeholder="fever, UTI signs, chills" /></label>
      <label>Body temperature<input value={item.bodyTemperature} onChange={(e) => set('bodyTemperature', e.target.value)} /></label>
      <DictationArea label="What helped?" value={item.treatmentResponse} onChange={(v) => set('treatmentResponse', v)} />
      <label>Relapse self-triage<select value={item.relapseFlag} onChange={(e) => set('relapseFlag', e.target.value)}>
        <option value="baseline">baseline fluctuation</option><option value="pseudo-flare">possible pseudo-flare</option><option value="possible-relapse">possible relapse</option><option value="likely-relapse">likely relapse</option><option value="uncertain">uncertain</option>
      </select></label>
      <DictationArea label="Notes" value={item.notes} onChange={(v) => set('notes', v)} />
      <button className="primary">Save symptom</button>
    </form>
  );
}

function DictationArea({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  const [listening, setListening] = useState(false);
  const supported = typeof window !== 'undefined' && ((window as any).SpeechRecognition || (window as any).webkitSpeechRecognition);
  const dictate = () => {
    const SpeechRecognition = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    if (!SpeechRecognition) return;
    const recognition = new SpeechRecognition();
    recognition.lang = 'en-US';
    recognition.interimResults = true;
    recognition.continuous = false;
    recognition.onresult = (event: any) => {
      const text = Array.from(event.results).map((r: any) => r[0].transcript).join(' ');
      onChange(`${value} ${text}`.trim());
    };
    recognition.onend = () => setListening(false);
    setListening(true);
    recognition.start();
  };
  return (
    <label>
      {label}
      <textarea value={value} onChange={(e) => onChange(e.target.value)} rows={4} />
      {supported && <button type="button" className="secondary inline" onClick={dictate}>{listening ? 'Listening...' : 'Dictate'}</button>}
    </label>
  );
}

function VoiceScreen({ run }: { run: (w: () => Promise<void>, s?: string) => void }) {
  const mediaRecorder = useRef<MediaRecorder | null>(null);
  const chunks = useRef<Blob[]>([]);
  const [recording, setRecording] = useState(false);
  const [note, setNote] = useState('');
  const start = async () => {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    chunks.current = [];
    const recorder = new MediaRecorder(stream);
    recorder.ondataavailable = (event) => chunks.current.push(event.data);
    recorder.onstop = () => stream.getTracks().forEach((track) => track.stop());
    mediaRecorder.current = recorder;
    recorder.start();
    setRecording(true);
  };
  const stopAndSave = () => run(async () => {
    const recorder = mediaRecorder.current;
    if (!recorder) return;
    await new Promise<void>((resolve) => {
      recorder.onstop = () => {
        recorder.stream.getTracks().forEach((track) => track.stop());
        resolve();
      };
      recorder.stop();
    });
    setRecording(false);
    const blob = new Blob(chunks.current, { type: 'audio/webm' });
    const form = new FormData();
    form.append('file', blob, `voice-note-${Date.now()}.webm`);
    form.append('kind', 'audio_memo');
    form.append('relatedType', 'voice_note');
    await api.uploadAttachment(form);
  }, 'Audio memo saved.');
  return (
    <section className="flow">
      <DictationArea label="Unstructured note" value={note} onChange={setNote} />
      <button className="primary" onClick={() => run(async () => {
        await api.saveSymptom({ ...blankSymptom(), category: 'voice note', symptom: note || 'Voice note', notes: note });
        setNote('');
      }, 'Voice note text saved.')}>Save text note</button>
      <button className={recording ? 'danger' : 'secondary'} onClick={() => recording ? stopAndSave() : run(start)}>
        {recording ? 'Stop and save audio' : 'Record audio memo'}
      </button>
      <p className="muted">Browser dictation may depend on your device. Audio memos are stored locally in encrypted Cortex storage.</p>
    </section>
  );
}

function ClinicianScreen({ run }: { run: (w: () => Promise<void>, s?: string) => void }) {
  const [summary, setSummary] = useState<any>(null);
  useEffect(() => {
    run(async () => setSummary(await api.clinicianSummary()));
  }, []);
  if (!summary) return <p>Loading summary...</p>;
  return (
    <section className="flow">
      <p className="muted">Generated {new Date(summary.generatedAt).toLocaleString()}</p>
      <SummaryBlock title="Recent check-ins" items={summary.checkins.map((x: DailyCheckIn) => `${x.entryDate}: burden ${x.overallBurden}, fatigue ${x.fatigue}, pain ${x.pain}, fog ${x.brainFog}`)} />
      <SummaryBlock title="Recent symptoms" items={summary.symptoms.map((x: SymptomEvent) => `${new Date(x.occurredAt).toLocaleDateString()}: ${x.category} - ${x.symptom} (${x.severity}/10)`)} />
      <SummaryBlock title="Recent medications" items={summary.medications.map((x: MedicationEvent) => `${new Date(x.takenAt).toLocaleDateString()}: ${x.name} ${x.dose}`)} />
    </section>
  );
}

function SummaryBlock({ title, items }: { title: string; items: string[] }) {
  return <section className="summary-block"><h2>{title}</h2>{items.length ? items.map((item) => <p key={item}>{item}</p>) : <p className="muted">No entries yet.</p>}</section>;
}

function SettingsScreen({ status, settings, setSettings, run, refresh }: { status: Status; settings: Settings; setSettings: (s: Settings) => void; run: (w: () => Promise<void>, s?: string) => void; refresh: () => Promise<void> }) {
  const [totp, setTotp] = useState('');
  const [backupConfirmed, setBackupConfirmed] = useState(false);
  const [warningAcknowledge, setWarningAcknowledge] = useState(false);
  const save = (next: Settings) => {
    setSettings(next);
    run(async () => { setSettings(await api.saveSettings(next)); }, 'Settings saved.');
  };
  return (
    <section className="flow">
      <label>Theme<select value={settings.theme} onChange={(e) => save({ ...settings, theme: e.target.value as Settings['theme'] })}>
        <option value="system">System</option><option value="light">Day</option><option value="dark">Night</option>
      </select></label>
      <label>Accent color<input type="color" value={settings.accentColor} onChange={(e) => save({ ...settings, accentColor: e.target.value })} /></label>
      <label>Text and control size<select value={settings.fontScale} onChange={(e) => save({ ...settings, fontScale: e.target.value as Settings['fontScale'] })}>
        <option value="compact">Compact</option><option value="comfortable">Comfortable</option><option value="large">Large</option><option value="extra-large">Extra large</option>
      </select></label>
      <label className="choice"><input type="checkbox" checked={settings.highContrast} onChange={(e) => save({ ...settings, highContrast: e.target.checked })} /> High contrast</label>
      <label className="choice"><input type="checkbox" checked={settings.reducedMotion} onChange={(e) => save({ ...settings, reducedMotion: e.target.checked })} /> Reduced motion</label>
      <button className="secondary" onClick={() => run(async () => {
        const options = await api.passkeyRegisterOptions();
        const credential = await createPasskey(options);
        await api.passkeyRegisterFinish(credential);
        await refresh();
      }, 'Passkey enrolled.')}>Enroll passkey</button>
      {status.passwordAuthEnabled && (
        <div className="danger-zone">
          <h2>Password login</h2>
          <p className="muted">Disable this only after your passkey and backup codes are confirmed.</p>
          <label className="choice"><input type="checkbox" checked={backupConfirmed} onChange={(e) => setBackupConfirmed(e.target.checked)} /> I have saved my 4 backup codes.</label>
          <label className="choice"><input type="checkbox" checked={warningAcknowledge} onChange={(e) => setWarningAcknowledge(e.target.checked)} /> I understand my encryption profile and recovery path.</label>
          <label>Authenticator code<input value={totp} onChange={(e) => setTotp(e.target.value)} inputMode="numeric" /></label>
          <button className="danger" onClick={() => run(async () => { await api.disablePassword(totp, backupConfirmed, warningAcknowledge); await refresh(); }, 'Password login disabled.')}>Disable password login</button>
        </div>
      )}
    </section>
  );
}
