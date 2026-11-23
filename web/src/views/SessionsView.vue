<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { auth, clearSession } from '../auth'
import { formatDate, formatRelative } from '../format'
import { t } from '../i18n'
import { notify } from '../toast'
import StatusBadge from '../components/StatusBadge.vue'

const router = useRouter()
const sessions = ref([])
const loading = ref(true)
const error = ref('')
const busyId = ref('')

const currentId = computed(() => auth.session?.id || '')
const otherCount = computed(() => sessions.value.filter((item) => !item.current).length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.sessions()
    sessions.value = data?.sessions || []
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function revoke(session) {
  const client = session.client || t('unknown')
  if (!window.confirm(t('Revoke the {client} session?', { client }))) return
  busyId.value = session.id
  try {
    await api.revokeSession(session.id)
    if (session.current) {
      clearSession()
      notify(t('Signed out.'), 'success')
      router.push({ name: 'login' })
      return
    }
    sessions.value = sessions.value.filter((item) => item.id !== session.id)
    notify(t('Session revoked.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busyId.value = ''
  }
}

async function revokeOthers() {
  if (otherCount.value === 0) return
  if (!window.confirm(t('Revoke {count} other session(s)?', { count: otherCount.value }))) return
  let failures = 0
  for (const session of sessions.value.filter((item) => !item.current)) {
    try {
      await api.revokeSession(session.id)
    } catch {
      failures += 1
    }
  }
  await load()
  if (failures) {
    notify(t('{count} session(s) could not be revoked.', { count: failures }), 'error')
  } else {
    notify(t('Other sessions revoked.'), 'success')
  }
}

onMounted(load)
</script>

<template>
  <div class="page-stack">
    <section class="page-heading">
      <div>
        <p class="eyebrow">{{ t('Security') }}</p>
        <h1>{{ t('Sessions') }}</h1>
        <p class="muted">{{ t('Devices and clients that can access your account.') }}</p>
      </div>
      <div class="heading-actions">
        <button class="btn btn-secondary btn-sm" type="button" @click="load" :disabled="loading">
          {{ t('Refresh') }}
        </button>
        <button
          class="btn btn-danger btn-sm"
          type="button"
          :disabled="loading || otherCount === 0"
          @click="revokeOthers"
        >
          {{ t('Revoke other sessions') }}
        </button>
      </div>
    </section>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <section class="card table-card">
      <div v-if="loading" class="loading-row">
        <span class="spinner spinner-sm" aria-hidden="true"></span>
        {{ t('Loading sessions…') }}
      </div>
      <div v-else-if="sessions.length === 0" class="empty-state">
        {{ t('No active sessions.') }}
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('Client') }}</th>
              <th>{{ t('Address') }}</th>
              <th>{{ t('Created') }}</th>
              <th>{{ t('Last used') }}</th>
              <th>{{ t('Expires') }}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="session in sessions" :key="session.id">
              <td>
                <div class="cell-stack">
                  <span>
                    {{ session.client || 'api' }}
                    <StatusBadge v-if="session.current" value="current" kind="status" />
                  </span>
                  <small :title="session.user_agent">
                    {{ session.user_agent || t('Unknown client') }}
                  </small>
                </div>
              </td>
              <td><code>{{ session.ip || '—' }}</code></td>
              <td>{{ formatDate(session.created_at) }}</td>
              <td>{{ formatRelative(session.last_used_at) }}</td>
              <td>{{ formatDate(session.expires_at) }}</td>
              <td class="cell-actions">
                <button
                  class="btn btn-ghost btn-sm"
                  type="button"
                  :disabled="busyId === session.id"
                  @click="revoke(session)"
                >
                  {{ t('Revoke') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
