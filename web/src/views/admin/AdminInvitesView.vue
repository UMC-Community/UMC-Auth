<script setup>
import { onMounted, reactive, ref } from 'vue'
import { api } from '../../api'
import { copyText, formatDate } from '../../format'
import { t } from '../../i18n'
import { notify } from '../../toast'
import AdminNav from '../../components/AdminNav.vue'

const invites = ref([])
const loading = ref(true)
const busy = ref('')
const error = ref('')
const newCodes = ref([])
const form = reactive({ count: 1, uses: 1, days: 30, note: '' })

const statusClass = {
  active: 'badge-verified',
  revoked: 'badge-status-disabled',
  expired: 'badge-warning',
  exhausted: 'badge-warning',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.admin.invites(200)
    invites.value = data?.invites || []
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function create() {
  busy.value = 'create'
  try {
    const data = await api.admin.createInvites({
      count: Number(form.count),
      uses: Number(form.uses),
      days: Number(form.days),
      note: form.note.trim(),
    })
    invites.value = data?.invites || []
    newCodes.value = data?.codes || []
    notify(t('{count} invite code(s) created.', { count: newCodes.value.length }), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function revoke(invite) {
  if (!window.confirm(t('Revoke invite {prefix}?', { prefix: invite.code_prefix }))) return
  busy.value = `invite:${invite.id}`
  try {
    await api.admin.revokeInvite(invite.id)
    invites.value = invites.value.map((item) =>
      item.id === invite.id ? { ...item, status: 'revoked' } : item,
    )
    notify(t('Invite revoked.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function copy(value, label = 'Invite code') {
  try {
    await copyText(value)
    notify(t('{label} copied', { label: t(label) }), 'success', 2200)
  } catch {
    notify(t('Could not copy to the clipboard'), 'error')
  }
}

async function copyAll() {
  if (!newCodes.value.length) return
  await copy(newCodes.value.join('\n'), 'Invite codes')
}

onMounted(load)
</script>

<template>
  <div class="page-stack">
    <AdminNav />
    <section class="page-heading">
      <div>
        <p class="eyebrow">{{ t('Administration') }}</p>
        <h1>{{ t('Invite codes') }}</h1>
        <p class="muted">
          {{ t('Create registration invites for invite-only mode. Full codes are shown once after creation.') }}
        </p>
      </div>
      <div class="heading-actions">
        <button class="btn btn-secondary btn-sm" type="button" :disabled="loading" @click="load">
          {{ t('Refresh') }}
        </button>
      </div>
    </section>

    <div v-if="newCodes.length" class="alert alert-warning generated-panel">
      <div>
        <strong>{{ t('New invite codes') }}</strong>
        <span class="muted small">{{ t('Shown once. Copy them now.') }}</span>
        <div class="codes">
          <code v-for="code in newCodes" :key="code">{{ code }}</code>
        </div>
      </div>
      <div class="generated-actions">
        <button class="btn btn-secondary btn-sm" type="button" @click="copyAll">
          {{ t('Copy all') }}
        </button>
        <button class="btn btn-ghost btn-sm" type="button" @click="newCodes = []">
          {{ t('Dismiss') }}
        </button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <section class="card">
      <h2>{{ t('Create invites') }}</h2>
      <form class="form-grid" @submit.prevent="create">
        <label class="field">
          <span>{{ t('Count') }}</span>
          <input v-model.number="form.count" type="number" min="1" max="50" required />
          <small>{{ t('1–50 codes per batch.') }}</small>
        </label>
        <label class="field">
          <span>{{ t('Uses per code') }}</span>
          <input v-model.number="form.uses" type="number" min="1" max="100" required />
        </label>
        <label class="field">
          <span>{{ t('Valid for days') }}</span>
          <input v-model.number="form.days" type="number" min="1" max="365" required />
        </label>
        <label class="field">
          <span>{{ t('Note') }}</span>
          <input v-model="form.note" maxlength="200" :placeholder="t('Optional batch note')" />
        </label>
        <div class="card-actions" style="grid-column: 1 / -1; margin-top: 0">
          <button class="btn btn-primary btn-sm" type="submit" :disabled="busy === 'create'">
            {{ busy === 'create' ? t('Creating…') : t('Create invites') }}
          </button>
        </div>
      </form>
    </section>

    <section class="card table-card">
      <h2>{{ t('Recent invites') }}</h2>
      <div v-if="loading" class="loading-row">
        <span class="spinner spinner-sm" aria-hidden="true"></span>
        {{ t('Loading invites…') }}
      </div>
      <div v-else-if="invites.length === 0" class="empty-state">
        {{ t('No invite codes yet.') }}
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('Code') }}</th>
              <th>{{ t('Note') }}</th>
              <th>{{ t('Uses') }}</th>
              <th>{{ t('Expires') }}</th>
              <th>{{ t('Status') }}</th>
              <th class="cell-actions">{{ t('Actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="invite in invites" :key="invite.id">
              <td><code class="code-inline">{{ invite.code_prefix }}…</code></td>
              <td>{{ invite.note || '—' }}</td>
              <td>{{ invite.used_count }} / {{ invite.max_uses }}</td>
              <td class="nowrap">{{ formatDate(invite.expires_at) }}</td>
              <td>
                <span class="badge" :class="statusClass[invite.status] || 'badge-warning'">
                  {{ t(invite.status) }}
                </span>
              </td>
              <td class="cell-actions">
                <div v-if="invite.status === 'active'" class="action-row">
                  <button
                    class="btn btn-ghost btn-xs"
                    type="button"
                    :disabled="busy === `invite:${invite.id}`"
                    @click="revoke(invite)"
                  >
                    {{ t('Revoke') }}
                  </button>
                </div>
                <span v-else class="muted small">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
