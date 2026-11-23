import { computed, reactive } from 'vue'
import {
  api,
  refreshSession,
  setAccessToken,
  setUnauthorizedHandler,
} from './api'

export const auth = reactive({
  user: null,
  session: null,
  ready: false,
  loading: false,
})

export const isAuthenticated = computed(() => Boolean(auth.user))
export const isAdmin = computed(
  () => Boolean(auth.user) && ['admin', 'superadmin'].includes(auth.user.role),
)
export const isStaff = computed(
  () => Boolean(auth.user) && ['moderator', 'admin', 'superadmin'].includes(auth.user.role),
)

export function setSession(data) {
  if (data?.access_token) setAccessToken(data.access_token)
  auth.user = data?.user || null
  auth.session = data?.session || null
}

export function clearSession() {
  setAccessToken(null)
  auth.user = null
  auth.session = null
}

setUnauthorizedHandler(clearSession)

export async function bootstrap() {
  if (auth.ready) return
  auth.loading = true
  try {
    const data = await refreshSession()
    if (data?.user) {
      auth.user = data.user
      auth.session = data.session
    } else {
      clearSession()
    }
  } finally {
    auth.loading = false
    auth.ready = true
  }
}

export async function logout() {
  try {
    await api.logout()
  } catch {
    // The cookie may already be gone; clearing local state is enough.
  }
  clearSession()
}

export async function refreshUser() {
  const data = await api.me()
  auth.user = data.user
  auth.session = data.session
  return data
}
