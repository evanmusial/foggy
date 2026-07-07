import type { DailyCheckIn, MedicationEvent, Settings, Status, SymptomEvent } from './types';

let csrfToken = '';

export function setCSRF(token: string) {
  csrfToken = token || csrfToken;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type') && !(init.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }
  if (csrfToken && !['GET', 'HEAD'].includes((init.method || 'GET').toUpperCase())) {
    headers.set('X-CSRF-Token', csrfToken);
  }
  const res = await fetch(path, { ...init, headers, credentials: 'same-origin' });
  const text = await res.text();
  const data = text ? JSON.parse(text) : {};
  if (!res.ok) {
    throw new Error(data.error || `Request failed: ${res.status}`);
  }
  return data as T;
}

export const api = {
  async status() {
    const status = await request<Status>('/api/status');
    setCSRF(status.csrfToken);
    return status;
  },
  async setup(input: { displayName: string; password: string; encryptionProfile: string; accentColor: string; theme: string }) {
    const data = await request<{ csrfToken: string; backupCodes: string[]; totpSecret: string; totpUrl: string }>('/api/setup', {
      method: 'POST',
      body: JSON.stringify(input)
    });
    setCSRF(data.csrfToken);
    return data;
  },
  async loginPassword(password: string, totpCode: string) {
    const data = await request<{ csrfToken: string }>('/api/auth/login-password', {
      method: 'POST',
      body: JSON.stringify({ password, totpCode })
    });
    setCSRF(data.csrfToken);
  },
  async recover(backupCode: string, newPassword: string) {
    const data = await request<{ csrfToken: string }>('/api/auth/recover', {
      method: 'POST',
      body: JSON.stringify({ backupCode, newPassword })
    });
    setCSRF(data.csrfToken);
  },
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  checkins: () => request<{ items: DailyCheckIn[] }>('/api/checkins'),
  saveCheckin: (item: DailyCheckIn) => request<DailyCheckIn>('/api/checkins', { method: 'POST', body: JSON.stringify(item) }),
  symptoms: () => request<{ items: SymptomEvent[] }>('/api/symptoms'),
  saveSymptom: (item: SymptomEvent) => request<SymptomEvent>('/api/symptoms', { method: 'POST', body: JSON.stringify(item) }),
  medications: () => request<{ items: MedicationEvent[] }>('/api/medications'),
  saveMedication: (item: MedicationEvent) => request<MedicationEvent>('/api/medications', { method: 'POST', body: JSON.stringify(item) }),
  settings: () => request<Settings>('/api/settings'),
  saveSettings: (settings: Settings) => request<Settings>('/api/settings', { method: 'POST', body: JSON.stringify(settings) }),
  clinicianSummary: () => request<{ generatedAt: string; checkins: DailyCheckIn[]; symptoms: SymptomEvent[]; medications: MedicationEvent[] }>('/api/exports/clinician-summary'),
  disablePassword: (totpCode: string, backupConfirmed: boolean, warningAcknowledge: boolean) =>
    request('/api/security/disable-password', {
      method: 'POST',
      body: JSON.stringify({ totpCode, backupConfirmed, warningAcknowledge })
    }),
  uploadAttachment: (form: FormData) => request('/api/attachments', { method: 'POST', body: form }),
  passkeyRegisterOptions: () => request<PublicKeyCredentialCreationOptionsJSON>('/api/passkeys/register/options', { method: 'POST' }),
  passkeyRegisterFinish: (credential: unknown) =>
    request('/api/passkeys/register/finish', { method: 'POST', body: JSON.stringify(credential) }),
  passkeyLoginOptions: () => request<PublicKeyCredentialRequestOptionsJSON>('/api/passkeys/login/options', { method: 'POST' }),
  passkeyLoginFinish: async (credential: unknown) => {
    const data = await request<{ csrfToken: string }>('/api/passkeys/login/finish', { method: 'POST', body: JSON.stringify(credential) });
    setCSRF(data.csrfToken);
  }
};

type PublicKeyCredentialCreationOptionsJSON = Record<string, unknown>;
type PublicKeyCredentialRequestOptionsJSON = Record<string, unknown>;
