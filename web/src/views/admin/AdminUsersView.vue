<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { api } from '../../api'
import { auth, isAdmin } from '../../auth'
import { copyText, formatDate } from '../../format'
import { t } from '../../i18n'
import { notify } from '../../toast'
import AdminNav from '../../components/AdminNav.vue'
import StatusBadge from '../../components/StatusBadge.vue'

const query = ref('')
const users = ref([])
const total = ref(0)
const limit = 25
const offset = ref(0)
const loading = ref(false)
const error = ref('')
const busy = ref('')
const generated = ref(null)
const roleDrafts = reactive({})

const page = computed(() => Math.floor(offset.value / limit) + 1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / limit)))
const roleOptions = computed(() =>
  auth.user?.role === 'superadmin'
    ? ['user', 'moderator', 'admin', 'superadmin']
    : ['user', 'moderator'],
)

function canManage(target) {
  if (!isAdmin.value) return false
  if (!target || target.uuid === auth.user?.uuid) return false
  if (auth.user?.role !== 'superadmin' && ['admin', 'superadmin'].includes(target.role)) {
    return false
  }
  return true
}

function applyUser(user) {
  const index = users.value.findIndex((item) => item.uuid === user.uuid)
  if (index >= 0) {
    users.value[index] = { ...users.value[index], ...user }
  }
  roleDrafts[user.uuid] = user.role
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await api.admin.users(query.value, limit, offset.value)
    users.value = data.users || []
    total.value = data.total || 0
    for (const user of users.value) {
      roleDrafts[user.uuid] = user.role
    }
  } catch (err) {
    error.value = err.message
  } finally {
    loading.value = false
  }
}

function search() {
  offset.value = 0
  load()
}

function go(delta) {
  const next = offset.value + delta * limit
  if (next < 0 || next >= total.value) return
  offset.value = next
  load()
}

async function toggleStatus(user) {
  const disable = user.status !== 'disabled'
  const prompt = disable
    ? t('Disable {username}?', { username: user.username })
    : t('Enable {username}?', { username: user.username })
  if (!window.confirm(prompt)) return
  busy.value = `${user.uuid}:status`
  try {
    const data = await api.admin.setStatus(user.uuid, disable)
    applyUser(data.user)
    notify(disable ? t('Account disabled.') : t('Account enabled.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function saveRole(user) {
  const role = roleDrafts[user.uuid]
  if (!role || role === user.role) return
  busy.value = `${user.uuid}:role`
  try {
    const data = await api.admin.setRole(user.uuid, role)
    applyUser(data.user)
    notify(t('Role updated.'), 'success')
  } catch (err) {
    roleDrafts[user.uuid] = user.role
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function verify(user) {
  const identity = user.email || user.username
  if (!window.confirm(t('Mark {email} as verified?', { email: identity }))) return
  busy.value = `${user.uuid}:verify`
  try {
    const data = await api.admin.verifyEmail(user.uuid)
    applyUser(data.user)
    notify(t('Email marked verified.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function reset(user) {
  if (!window.confirm(t('Generate a new password for {username}?', { username: user.username }))) {
    return
  }
  busy.value = `${user.uuid}:reset`
  try {
    const data = await api.admin.resetPassword(user.uuid)
    applyUser(data.user)
    generated.value = {
      username: user.username,
      password: data.generated_password,
    }
    notify(t('Password reset. Copy the generated password now.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function copyGenerated() {
  if (!generated.value) return
  try {
    await copyText(generated.value.password)
    notify(t('Password copied'), 'success', 2200)
  } catch {
    notify(t('Could not copy the password'), 'error')
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
        <h1>{{ t('Users') }}</h1>
        <p class="muted">
          {{ t('Search accounts, change roles, disable users, and reset credentials.') }}
        </p>
      </div>
      <form class="search-form" @submit.prevent="search">
        <input
          v-model.trim="query"
          type="search"
          :placeholder="t('Username, email, or UUID')"
          :aria-label="t('Search users')"
        />
        <button class="btn btn-primary btn-sm" type="submit" :disabled="loading">
          {{ t('Search') }}
        </button>
      </form>
    </section>

    <div v-if="generated" class="alert alert-warning generated-panel">
      <div>
        {{ t('New password for') }} <strong>{{ generated.username }}</strong>:
        <code>{{ generated.password }}</code>
        <span class="muted small">{{ t('Shown once. Copy it now.') }}</span>
      </div>
      <div class="generated-actions">
        <button class="btn btn-secondary btn-sm" type="button" @click="copyGenerated">
          {{ t('Copy') }}
        </button>
        <button class="btn btn-ghost btn-sm" type="button" @click="generated = null">
          {{ t('Dismiss') }}
        </button>
      </div>
    </div>

    <div v-if="error" class="alert alert-error">{{ error }}</div>

    <section class="card table-card">
      <div v-if="loading" class="loading-row">
        <span class="spinner spinner-sm" aria-hidden="true"></span>
        {{ t('Loading users…') }}
      </div>
      <div v-else-if="users.length === 0" class="empty-state">
        {{ t('No accounts matched this search.') }}
      </div>
      <div v-else class="table-wrap">
        <table class="data-table">
          <thead>
            <tr>
              <th>{{ t('Account') }}</th>
              <th>{{ t('Role') }}</th>
              <th>{{ t('Status') }}</th>
              <th>{{ t('Email') }}</th>
              <th>{{ t('Created') }}</th>
              <th class="cell-actions">{{ t('Actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in users" :key="user.uuid">
              <td>
                <div class="cell-stack">
                  <strong>{{ user.username }}</strong>
                  <small>{{ user.email || t('no email') }}</small>
                  <small class="mono">{{ user.uuid }}</small>
                </div>
              </td>
              <td>
                <div v-if="canManage(user)" class="role-editor">
                  <select
                    v-model="roleDrafts[user.uuid]"
                    :aria-label="t('Role for {username}', { username: user.username })"
                  >
                    <option v-for="role in roleOptions" :key="role" :value="role">
                      {{ t(role) }}
                    </option>
                  </select>
                  <button
                    class="btn btn-ghost btn-xs"
                    type="button"
                    :disabled="busy === `${user.uuid}:role` || roleDrafts[user.uuid] === user.role"
                    @click="saveRole(user)"
                  >
                    {{ t('Save') }}
                  </button>
                </div>
                <StatusBadge v-else :value="user.role" kind="role" />
              </td>
              <td><StatusBadge :value="user.status" kind="status" /></td>
              <td>
                <span v-if="user.email_verified" class="badge badge-verified">
                  {{ t('verified') }}
                </span>
                <span v-else class="badge badge-warning">{{ t('pending') }}</span>
              </td>
              <td>{{ formatDate(user.created_at) }}</td>
              <td class="cell-actions">
                <div v-if="canManage(user)" class="action-row">
                  <button
                    class="btn btn-ghost btn-xs"
                    type="button"
                    :disabled="busy === `${user.uuid}:status`"
                    @click="toggleStatus(user)"
                  >
                    {{ t(user.status === 'disabled' ? 'Enable' : 'Disable') }}
                  </button>
                  <button
                    v-if="!user.email_verified"
                    class="btn btn-ghost btn-xs"
                    type="button"
                    :disabled="busy === `${user.uuid}:verify`"
                    @click="verify(user)"
                  >
                    {{ t('Verify') }}
                  </button>
                  <button
                    class="btn btn-ghost btn-xs"
                    type="button"
                    :disabled="busy === `${user.uuid}:reset`"
                    @click="reset(user)"
                  >
                    {{ t('Reset password') }}
                  </button>
                </div>
                <span v-else class="muted small">
                  {{ t(user.uuid === auth.user?.uuid ? 'This is you' : 'Read only') }}
                </span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="table-footer">
        <span class="muted small">
          {{ t('{count} account(s) · page {page} of {pages}', {
            count: total,
            page,
            pages,
          }) }}
        </span>
        <div class="heading-actions">
          <button
            class="btn btn-ghost btn-sm"
            type="button"
            :disabled="offset === 0 || loading"
            @click="go(-1)"
          >
            {{ t('Previous') }}
          </button>
          <button
            class="btn btn-ghost btn-sm"
            type="button"
            :disabled="offset + limit >= total || loading"
            @click="go(1)"
          >
            {{ t('Next') }}
          </button>
        </div>
      </div>
    </section>
  </div>
</template>
