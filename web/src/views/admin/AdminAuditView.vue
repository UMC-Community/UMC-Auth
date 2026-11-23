<script setup>
import { onMounted, ref } from 'vue'
import { api } from '../../api'
import { formatDate } from '../../format'
import { t } from '../../i18n'
import AdminNav from '../../components/AdminNav.vue'

const entries = ref([])
const loading = ref(true)
const error = ref('')
const limit = ref(200)
const query = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.admin.audit(query.value.trim(), limit.value)
    entries.value = data?.entries || []
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
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
        <h1>{{ t('Audit log') }}</h1>
        <p class="muted">{{ t('Recent administrative and security-relevant actions.') }}</p>
      </div>
      <div class="heading-actions">
        <form class="search-form" @submit.prevent="load">
          <input
            v-model="query"
            type="search"
            :placeholder="t('Actor, action, or detail')"
            :aria-label="t('Search audit log')"
          />
          <button class="btn btn-primary btn-sm" type="submit" :disabled="loading">
            {{ t('Search') }}
          </button>
        </form>
        <label class="inline-field">
          <span>{{ t('Limit') }}</span>
          <select v-model.number="limit" @change="load">
            <option :value="50">50</option>
            <option :value="100">100</option>
            <option :value="200">200</option>
            <option :value="500">500</option>
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
        {{ t('Loading audit entries…') }}
      </div>
      <div v-else-if="entries.length === 0" class="empty-state">
        {{ t(query ? 'No audit entries matched this search.' : 'No audit entries yet.') }}
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('Time') }}</th>
              <th>{{ t('Actor') }}</th>
              <th>{{ t('Action') }}</th>
              <th>{{ t('Detail') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="entry in entries" :key="entry.id">
              <td class="nowrap">{{ formatDate(entry.created_at) }}</td>
              <td><strong>{{ entry.actor }}</strong></td>
              <td><code class="code-inline">{{ t(entry.action) }}</code></td>
              <td class="detail-cell">{{ entry.detail || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
