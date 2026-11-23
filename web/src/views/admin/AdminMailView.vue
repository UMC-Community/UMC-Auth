<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../../api'
import { formatDate, formatRelative } from '../../format'
import { t } from '../../i18n'
import { notify } from '../../toast'
import AdminNav from '../../components/AdminNav.vue'
import StatusBadge from '../../components/StatusBadge.vue'

const entries = ref([])
const loading = ref(true)
const error = ref('')
const status = ref('')
const busy = ref(0)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.admin.mail(status.value, 200)
    entries.value = data?.entries || []
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

async function retry(entry) {
  busy.value = entry.id
  try {
    await api.admin.retryMail(entry.id)
    notify(t('Message queued for retry.'), 'success')
    await load()
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = 0
  }
}

onMounted(load)
</script>

<template>
  <div class="page-stack">
    <AdminNav />
    <section class="page-heading">
      <div>
        <p class="eyebrow">{{ t('Administration') }}</p>
        <h1>{{ t('Mail outbox') }}</h1>
        <p class="muted">
          {{ t('Verification and password reset messages queued for delivery.') }}
        </p>
      </div>
      <div class="heading-actions">
        <label class="inline-field">
          <span>{{ t('Status') }}</span>
          <select v-model="status" @change="load">
            <option value="">{{ t('All') }}</option>
            <option value="pending">{{ t('Pending') }}</option>
            <option value="failed">{{ t('Failed') }}</option>
            <option value="sent">{{ t('Sent') }}</option>
          </select>
        </label>
        <button class="btn btn-secondary btn-sm" type="button" @click="load" :disabled="loading">
          {{ t('Refresh') }}
        </button>
      </div>
    </section>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <section class="card table-card">
      <div v-if="loading" class="loading-row">
        <span class="spinner spinner-sm" aria-hidden="true"></span>
        {{ t('Loading mail entries…') }}
      </div>
      <div v-else-if="entries.length === 0" class="empty-state">
        {{ t('No mail entries.') }}
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('Recipient') }}</th>
              <th>{{ t('Kind') }}</th>
              <th>{{ t('Status') }}</th>
              <th>{{ t('Attempts') }}</th>
              <th>{{ t('Next attempt') }}</th>
              <th>{{ t('Last error') }}</th>
              <th>{{ t('Created') }}</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in entries" :key="entry.id">
              <td><strong>{{ entry.recipient }}</strong></td>
              <td><code class="code-inline">{{ t(entry.kind) }}</code></td>
              <td><StatusBadge :value="entry.status" kind="status" /></td>
              <td>{{ entry.attempts }}</td>
              <td>
                {{ entry.status === 'sent' ? '—' : formatRelative(entry.next_attempt_at) }}
              </td>
              <td class="detail-cell" :title="entry.last_error">
                {{ entry.last_error || '—' }}
              </td>
              <td>{{ formatDate(entry.created_at) }}</td>
              <td class="cell-actions">
                <button
                  class="btn btn-ghost btn-xs"
                  type="button"
                  :disabled="busy === entry.id || entry.status === 'sent'"
                  @click="retry(entry)"
                >
                  {{ t('Retry') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
