<script setup>
import { computed, onMounted, ref } from 'vue'
import { api } from '../api'
import { auth, refreshUser } from '../auth'
import { config } from '../config'
import { copyText, formatDate, prettyUuid } from '../format'
import { t } from '../i18n'
import { notify } from '../toast'
import StatusBadge from '../components/StatusBadge.vue'

const user = computed(() => auth.user)
const refreshing = ref(false)
const profile = ref(null)
const profileLoading = ref(true)
const profileError = ref('')
const skinModel = ref('classic')
const busy = ref('')

const yggdrasilUrl = computed(
  () => config.meta?.yggdrasil_url || `${config.meta?.auth_origin || ''}/api/yggdrasil`,
)

const skinStyle = computed(() => {
  if (!profile.value?.skin_url) return {}
  return { backgroundImage: `url("${profile.value.skin_url}")` }
})

async function copy(value, label) {
  try {
    await copyText(value)
    notify(t('{label} copied', { label: t(label) }), 'success', 2200)
  } catch {
    notify(t('Could not copy to the clipboard'), 'error')
  }
}

async function refresh() {
  refreshing.value = true
  try {
    await Promise.all([refreshUser(), loadProfile()])
    notify(t('Profile refreshed.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    refreshing.value = false
  }
}

async function loadProfile() {
  profileLoading.value = true
  profileError.value = ''
  try {
    const data = await api.minecraft()
    profile.value = data?.profile || null
    skinModel.value = profile.value?.skin_model || 'classic'
  } catch (err) {
    profileError.value = err.message
  } finally {
    profileLoading.value = false
  }
}

function selectedFile(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  return file
}

async function uploadSkin(event) {
  const file = selectedFile(event)
  if (!file) return
  busy.value = 'skin'
  try {
    const data = await api.uploadSkin(file, skinModel.value)
    profile.value = data.profile
    notify(t('Skin updated.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function removeSkin() {
  if (!window.confirm(t('Remove your current skin?'))) return
  busy.value = 'skin'
  try {
    const data = await api.deleteSkin()
    profile.value = data.profile
    notify(t('Skin removed.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function uploadCape(event) {
  const file = selectedFile(event)
  if (!file) return
  busy.value = 'cape'
  try {
    const data = await api.uploadCape(file)
    profile.value = data.profile
    notify(t('Cape updated.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

async function removeCape() {
  if (!window.confirm(t('Remove your current cape?'))) return
  busy.value = 'cape'
  try {
    const data = await api.deleteCape()
    profile.value = data.profile
    notify(t('Cape removed.'), 'success')
  } catch (err) {
    notify(err.message, 'error')
  } finally {
    busy.value = ''
  }
}

onMounted(loadProfile)
</script>

<template>
  <div class="page-stack" v-if="user">
    <section class="page-heading">
      <div>
        <p class="eyebrow">{{ t('Account') }}</p>
        <h1>{{ t('Profile') }}</h1>
        <p class="muted">{{ t('Your identity across the website, launcher, and game server.') }}</p>
      </div>
      <div class="heading-actions">
        <button class="btn btn-secondary btn-sm" type="button" :disabled="refreshing" @click="refresh">
          {{ refreshing ? t('Refreshing…') : t('Refresh') }}
        </button>
      </div>
    </section>

    <div class="card-grid">
      <section class="card">
        <h2>{{ t('Identity') }}</h2>
        <dl class="detail-list">
          <div><dt>{{ t('Username') }}</dt><dd>{{ user.username }}</dd></div>
          <div><dt>{{ t('Email') }}</dt><dd>{{ user.email || '—' }}</dd></div>
          <div>
            <dt>{{ t('Email status') }}</dt>
            <dd>
              <span v-if="user.email_verified" class="badge badge-verified">{{ t('verified') }}</span>
              <span v-else class="badge badge-warning">{{ t('not verified') }}</span>
            </dd>
          </div>
          <div>
            <dt>{{ t('Role') }}</dt>
            <dd><StatusBadge :value="user.role" kind="role" /></dd>
          </div>
          <div>
            <dt>{{ t('Status') }}</dt>
            <dd><StatusBadge :value="user.status" kind="status" /></dd>
          </div>
        </dl>
      </section>

      <section class="card">
        <h2>{{ t('Minecraft profile') }}</h2>
        <dl class="detail-list">
          <div>
            <dt>{{ t('UUID') }}</dt>
            <dd>
              <code>{{ prettyUuid(user.uuid) }}</code>
              <button class="btn btn-ghost btn-xs" type="button" @click="copy(user.uuid, 'UUID')">
                {{ t('Copy') }}
              </button>
            </dd>
          </div>
          <div>
            <dt>{{ t('Launcher endpoint') }}</dt>
            <dd class="copy-row">
              <code class="code-wrap">{{ yggdrasilUrl }}</code>
              <button
                class="btn btn-ghost btn-xs"
                type="button"
                @click="copy(yggdrasilUrl, 'Yggdrasil URL')"
              >
                {{ t('Copy') }}
              </button>
            </dd>
          </div>
        </dl>
        <p class="muted small">
          {{ t('The launcher receives a signed textures property when it joins a server.') }}
        </p>
      </section>

      <section class="card">
        <h2>{{ t('Skin') }}</h2>
        <div v-if="profileError" class="alert alert-error">{{ profileError }}</div>
        <div v-else-if="profileLoading" class="loading-row">
          <span class="spinner spinner-sm" aria-hidden="true"></span>
          {{ t('Loading skin…') }}
        </div>
        <div v-else class="skin-editor">
          <div class="skin-preview-grid">
            <div v-if="profile?.skin_url" class="skin-face" :style="skinStyle" aria-hidden="true"></div>
            <div v-else class="skin-face skin-empty">?</div>
            <img
              v-if="profile?.skin_url"
              :src="profile.skin_url"
              class="skin-full"
              :alt="t('Current skin')"
            />
            <span v-else class="muted small">{{ t('No skin uploaded.') }}</span>
          </div>
          <div class="form-stack">
            <label class="field">
              <span>{{ t('Skin file') }}</span>
              <input
                type="file"
                accept="image/png"
                :disabled="busy === 'skin'"
                @change="uploadSkin"
              />
              <small>{{ t('64×64 or 64×32 PNG, up to 1 MB.') }}</small>
            </label>
            <label class="inline-field">
              <span>{{ t('Model') }}</span>
              <select v-model="skinModel" :disabled="busy === 'skin' || !profile">
                <option value="classic">{{ t('classic') }}</option>
                <option value="slim">{{ t('slim') }}</option>
              </select>
              <small>{{ t('Applied on the next skin upload.') }}</small>
            </label>
            <div class="card-actions">
              <button
                class="btn btn-ghost btn-sm"
                type="button"
                :disabled="busy === 'skin' || !profile?.skin_url"
                @click="removeSkin"
              >
                {{ t('Remove skin') }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="card">
        <h2>{{ t('Cape') }}</h2>
        <div class="skin-editor">
          <div class="skin-preview-grid">
            <img
              v-if="profile?.cape_url"
              :src="profile.cape_url"
              class="cape-preview"
              :alt="t('Current cape')"
            />
            <span v-else class="muted small">{{ t('No cape uploaded.') }}</span>
          </div>
          <div class="form-stack">
            <label class="field">
              <span>{{ t('Cape file') }}</span>
              <input
                type="file"
                accept="image/png"
                :disabled="busy === 'cape'"
                @change="uploadCape"
              />
              <small>{{ t('64×32 or 64×64 PNG, up to 1 MB.') }}</small>
            </label>
            <div class="card-actions">
              <button
                class="btn btn-ghost btn-sm"
                type="button"
                :disabled="busy === 'cape' || !profile?.cape_url"
                @click="removeCape"
              >
                {{ t('Remove cape') }}
              </button>
            </div>
          </div>
        </div>
      </section>

      <section class="card">
        <h2>{{ t('Account details') }}</h2>
        <dl class="detail-list">
          <div><dt>{{ t('Created') }}</dt><dd>{{ formatDate(user.created_at) }}</dd></div>
          <div><dt>{{ t('Updated') }}</dt><dd>{{ formatDate(user.updated_at) }}</dd></div>
          <div><dt>{{ t('Session') }}</dt><dd>{{ auth.session?.client || 'web' }}</dd></div>
          <div><dt>{{ t('Last used') }}</dt><dd>{{ formatDate(auth.session?.last_used_at) }}</dd></div>
        </dl>
      </section>

      <section class="card">
        <h2>{{ t('Account changes') }}</h2>
        <p class="muted">
          {{ t('Email changes and self-service account deletion are not available yet. Contact a server administrator if you need help with account recovery.') }}
        </p>
        <div class="card-actions">
          <RouterLink class="btn btn-secondary btn-sm" to="/sessions">
            {{ t('Review sessions') }}
          </RouterLink>
          <RouterLink class="btn btn-ghost btn-sm" to="/forgot-password">
            {{ t('Reset password') }}
          </RouterLink>
        </div>
      </section>
    </div>
  </div>
</template>
