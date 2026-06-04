const TOKEN_KEY = 'dionysus-api-token';

export function readToken() {
  return localStorage.getItem(TOKEN_KEY) || '';
}

export function writeToken(token) {
  localStorage.setItem(TOKEN_KEY, token.trim());
}

export async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  const token = readToken();
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(path, { ...options, headers });
  const payload = await response.json().catch(() => ({ data: null, errors: { response: response.statusText } }));
  if (!response.ok) {
    const message = payload?.errors ? Object.values(payload.errors).join(', ') : response.statusText;
    throw new Error(message || `HTTP ${response.status}`);
  }
  return payload.data;
}

export function postJSON(path, body) {
  return api(path, {
    method: 'POST',
    body: JSON.stringify(body)
  });
}
