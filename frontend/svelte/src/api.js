const TOKEN_KEY = 'dionysus-jwt-token';

export function readToken() {
  return localStorage.getItem(TOKEN_KEY) || '';
}

export function writeToken(token) {
  localStorage.setItem(TOKEN_KEY, token.trim());
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export async function login(username, password) {
  const data = await request('/api2/json/access/ticket', {
    method: 'POST',
    skipAuth: true,
    body: JSON.stringify({ username, password })
  });
  writeToken(data.token || '');
  return data;
}

export async function api(path, options = {}) {
  return request(path, options);
}

async function request(path, options = {}) {
  const { skipAuth, ...fetchOptions } = options;
  const headers = new Headers(options.headers || {});
  const token = readToken();
  if (token && !skipAuth) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(path, { ...fetchOptions, headers });
  const payload = await response.json().catch(() => ({ data: null, errors: { response: response.statusText } }));
  if (!response.ok) {
    const message = payload?.errors ? Object.values(payload.errors).join(', ') : response.statusText;
    const error = new Error(message || `HTTP ${response.status}`);
    error.status = response.status;
    error.payload = payload;
    throw error;
  }
  return payload.data;
}

export function postJSON(path, body, options = {}) {
	return api(path, {
		...options,
		method: 'POST',
		body: JSON.stringify(body)
	});
}
