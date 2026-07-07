export type Status = {
  initialized: boolean;
  unlocked: boolean;
  authenticated: boolean;
  encryptionProfile: string;
  passwordAuthEnabled: boolean;
  passkeysEnabled: boolean;
  csrfToken: string;
  displayName: string;
  requireHttps: boolean;
};

export type Settings = {
  theme: 'system' | 'light' | 'dark';
  accentColor: string;
  fontScale: 'compact' | 'comfortable' | 'large' | 'extra-large';
  highContrast: boolean;
  reducedMotion: boolean;
};

export type DailyCheckIn = {
  id?: string;
  entryDate: string;
  overallBurden: number;
  fatigue: number;
  energy: number;
  pain: number;
  mood: number;
  anxiety: number;
  brainFog: number;
  sleepQuality: number;
  heatSensitivity: number;
  mobility: number;
  bladderBowel: number;
  notes: string;
  createdAt?: string;
  updatedAt?: string;
};

export type SymptomEvent = {
  id?: string;
  occurredAt: string;
  category: string;
  symptom: string;
  bodyLocation: string;
  severity: number;
  duration: string;
  newness: string;
  functionImpact: string;
  heatExposure: string;
  triggers: string[];
  infectionSigns: string;
  bodyTemperature: string;
  treatmentResponse: string;
  relapseFlag: string;
  notes: string;
  createdAt?: string;
};

export type MedicationEvent = {
  id?: string;
  takenAt: string;
  name: string;
  dose: string;
  reason: string;
  effectiveness: string;
  sideEffects: string;
  createdAt?: string;
};
