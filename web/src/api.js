import { apiErrorText } from './i18n'

let accessToken = null
let unauthorizedHandler = null
let refreshPromise = null

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export function setAccessToken(token) {
  accessToken = token || null
}

export function setUnauthorizedHandler(handler) {
  unauthorizedHandler = handler
}

function notifyUnauthorized() {
  accessToken = null
  if (unauthorizedHandler) unauthorizedHandler()
}

function hasSessionHint() {
  return document.cookie
    .split('; ')
    .some((item) => item === 'umc_auth_session_hint=1')
}

function clearSessionHint() {
  document.cookie = 'umc_auth_session_hint=; Max-Age=0; Path=/; SameSite=Lax'
}

async function decode(response) {
  if (response.status === 204) return null
  const text = await response.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return null
  }
}

async function request(path, { method = 'GET', body, form, auth = true, retry = true } = {}) {
  const headers = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (auth && accessToken) headers.Authorization = `Bearer ${accessToken}`

  let response
  try {
    response = await fetch(`/api/v1${path}`, {
      method,
      headers,
      body: form !== undefined ? form : body === undefined ? undefined : JSON.stringify(body),
      credentials: 'same-origin',
    })
  } catch {
    throw new ApiError(
      0,
      'network_error',
      apiErrorText('network_error', 'Could not reach the authentication service'),
    )
  }

  const data = await decode(response)

  if (response.status === 401 && auth && retry) {
    const refreshed = await refreshSession()
    if (refreshed) return request(path, { method, body, form, auth, retry: false })
  }

  if (!response.ok) {
    const apiError = data?.error || {}
    const code = apiError.code || 'request_failed'
    throw new ApiError(
      response.status,
      code,
      apiErrorText(code, apiError.message || `Request failed (${response.status})`),
    )
  }
  return data
}

export async function refreshSession() {
  if (!hasSessionHint()) return null
  if (refreshPromise) return refreshPromise
  refreshPromise = (async () => {
    try {
      const response = await fetch('/api/v1/auth/refresh', {
        method: 'POST',
        headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
        body: '{}',
        credentials: 'same-origin',
      })
      if (!response.ok) {
        clearSessionHint()
        notifyUnauthorized()
        return null
      }
      const data = await decode(response)
      if (!data?.access_token) {
        clearSessionHint()
        notifyUnauthorized()
        return null
      }
      accessToken = data.access_token
      return data
    } catch {
      clearSessionHint()
      notifyUnauthorized()
      return null
    } finally {
      refreshPromise = null
    }
  })()
  return refreshPromise
}

export async function registerChallenge() {
  let response
  try {
    response = await fetch('/api/register/challenge', {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
    })
  } catch {
    throw new ApiError(
      0,
      'network_error',
      apiErrorText('network_error', 'Could not reach the authentication service'),
    )
  }
  const data = await decode(response)
  if (!response.ok) {
    throw new ApiError(
      response.status,
      data?.error || 'challenge_failed',
      apiErrorText(
        data?.error,
        data?.errorMessage || 'Could not get a proof-of-work challenge',
      ),
    )
  }
  return data
}

export const api = {
  meta: () => request('/meta', { auth: false }),

  register: (payload) =>
    request('/auth/register', {
      method: 'POST',
      auth: false,
      body: { ...payload, client: 'web' },
    }),
  verifyEmail: (email, code) =>
    request('/auth/verify-email', {
      method: 'POST',
      auth: false,
      body: { email, code, client: 'web' },
    }),
  resendVerification: (email) =>
    request('/auth/verify-email/resend', { method: 'POST', auth: false, body: { email } }),
  login: (login, password) =>
    request('/auth/login', {
      method: 'POST',
      auth: false,
      body: { login, password, client: 'web' },
    }),
  logout: () =>
    request('/auth/logout', { method: 'POST', auth: false, retry: false, body: {} }),
  forgotPassword: (email) =>
    request('/auth/password/forgot', { method: 'POST', auth: false, body: { email } }),
  resetPassword: (email, code, newPassword) =>
    request('/auth/password/reset', {
      method: 'POST',
      auth: false,
      body: { email, code, new_password: newPassword },
    }),

  me: () => request('/users/me'),
  sessions: () => request('/users/me/sessions'),
  revokeSession: (id) =>
    request(`/users/me/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' }),

  minecraft: () => request('/users/me/minecraft'),
  uploadSkin: (file, model = 'classic') => {
    const form = new FormData()
    form.append('file', file)
    form.append('model', model)
    return request('/users/me/skin', { method: 'POST', form })
  },
  deleteSkin: () => request('/users/me/skin', { method: 'DELETE' }),
  uploadCape: (file) => {
    const form = new FormData()
    form.append('file', file)
    return request('/users/me/cape', { method: 'POST', form })
  },
  deleteCape: () => request('/users/me/cape', { method: 'DELETE' }),

  admin: {
    stats: () => request('/admin/stats'),
    users: (query = '', limit = 25, offset = 0) =>
      request(
        `/admin/users?query=${encodeURIComponent(query)}&limit=${limit}&offset=${offset}`,
      ),
    user: (uuid) => request(`/admin/users/${encodeURIComponent(uuid)}`),
    setStatus: (uuid, disabled) =>
      request(`/admin/users/${encodeURIComponent(uuid)}/status`, {
        method: 'POST',
        body: { disabled },
      }),
    setRole: (uuid, role) =>
      request(`/admin/users/${encodeURIComponent(uuid)}/role`, {
        method: 'POST',
        body: { role },
      }),
    verifyEmail: (uuid) =>
      request(`/admin/users/${encodeURIComponent(uuid)}/verify-email`, { method: 'POST' }),
    resetPassword: (uuid) =>
      request(`/admin/users/${encodeURIComponent(uuid)}/reset-password`, {
        method: 'POST',
        body: {},
      }),
    audit: (query = '', limit = 100) =>
      request(`/admin/audit?query=${encodeURIComponent(query)}&limit=${limit}`),
    mail: (status = '', limit = 100) =>
      request(`/admin/mail?status=${encodeURIComponent(status)}&limit=${limit}`),
    retryMail: (id) =>
      request(`/admin/mail/${encodeURIComponent(id)}/retry`, { method: 'POST' }),
    servers: (limit = 100) => request(`/admin/servers?limit=${limit}`),
    invites: (limit = 100) => request(`/admin/invites?limit=${limit}`),
    createInvites: (payload) =>
      request('/admin/invites', { method: 'POST', body: payload }),
    revokeInvite: (id) =>
      request(`/admin/invites/${encodeURIComponent(id)}/revoke`, { method: 'POST' }),
    createServer: (payload) =>
      request('/admin/servers', { method: 'POST', body: payload }),
    setServerStatus: (id, status) =>
      request(`/admin/servers/${encodeURIComponent(id)}/status`, {
        method: 'POST',
        body: { status },
      }),
    deleteServer: (id) =>
      request(`/admin/servers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    serverJoins: (serverId = '', limit = 100) =>
      request(
        `/admin/servers/joins?server_id=${encodeURIComponent(serverId)}&limit=${limit}`,
      ),
  },
}
