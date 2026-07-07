function base64urlToBuffer(value: string): ArrayBuffer {
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(value.length / 4) * 4, '=');
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
  return bytes.buffer;
}

function bufferToBase64url(value: ArrayBuffer): string {
  const bytes = new Uint8Array(value);
  let binary = '';
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function creationOptions(raw: any): PublicKeyCredentialCreationOptions {
  const source = raw.publicKey ?? raw;
  const parser = (PublicKeyCredential as any).parseCreationOptionsFromJSON;
  if (parser) return parser(source);
  return {
    ...source,
    challenge: base64urlToBuffer(source.challenge),
    user: { ...source.user, id: base64urlToBuffer(source.user.id) },
    excludeCredentials: source.excludeCredentials?.map((c: any) => ({ ...c, id: base64urlToBuffer(c.id) }))
  };
}

function requestOptions(raw: any): PublicKeyCredentialRequestOptions {
  const source = raw.publicKey ?? raw;
  const parser = (PublicKeyCredential as any).parseRequestOptionsFromJSON;
  if (parser) return parser(source);
  return {
    ...source,
    challenge: base64urlToBuffer(source.challenge),
    allowCredentials: source.allowCredentials?.map((c: any) => ({ ...c, id: base64urlToBuffer(c.id) }))
  };
}

function credentialToJSON(credential: PublicKeyCredential): unknown {
  const modern = (credential as any).toJSON;
  if (modern) return modern.call(credential);
  const response = credential.response as AuthenticatorAttestationResponse | AuthenticatorAssertionResponse;
  const out: any = {
    id: credential.id,
    rawId: bufferToBase64url(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: bufferToBase64url(response.clientDataJSON)
    }
  };
  if ('attestationObject' in response) {
    out.response.attestationObject = bufferToBase64url(response.attestationObject);
    out.response.transports = response.getTransports?.() ?? [];
  } else {
    out.response.authenticatorData = bufferToBase64url(response.authenticatorData);
    out.response.signature = bufferToBase64url(response.signature);
    out.response.userHandle = response.userHandle ? bufferToBase64url(response.userHandle) : null;
  }
  return out;
}

export async function createPasskey(options: unknown): Promise<unknown> {
  if (!window.PublicKeyCredential) throw new Error('Passkeys are not available in this browser.');
  const credential = (await navigator.credentials.create({ publicKey: creationOptions(options) })) as PublicKeyCredential | null;
  if (!credential) throw new Error('Passkey enrollment was cancelled.');
  return credentialToJSON(credential);
}

export async function getPasskey(options: unknown): Promise<unknown> {
  if (!window.PublicKeyCredential) throw new Error('Passkeys are not available in this browser.');
  const credential = (await navigator.credentials.get({ publicKey: requestOptions(options) })) as PublicKeyCredential | null;
  if (!credential) throw new Error('Passkey login was cancelled.');
  return credentialToJSON(credential);
}
