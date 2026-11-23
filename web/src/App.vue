<script setup>
import { computed, onMounted, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { auth, isAdmin, isStaff, logout } from './auth'
import { config, loadMeta } from './config'
import { locale, t } from './i18n'
import LanguageSwitcher from './components/LanguageSwitcher.vue'
import ToastHost from './components/ToastHost.vue'

const route = useRoute()
const router = useRouter()

const serverName = computed(() => config.meta?.server_name || 'Account service')
const initial = computed(() => (auth.user?.username || '?').slice(0, 1).toUpperCase())
const minimalLayout = computed(() =>
  ['login', 'register', 'verify-email', 'forgot-password', 'reset-password'].includes(
    route.name,
  ),
)

watch(locale, () => {
  document.title = route.meta?.title ? t(route.meta.title) : 'UMC Auth'
})

async function signOut() {
  await logout()
  router.push({ name: 'login' })
}

onMounted(() => {
  loadMeta()
})
</script>

<template>
  <a class="skip-link" href="#main">{{ t('Skip to content') }}</a>

  <header v-if="!minimalLayout" class="site-header">
    <div class="shell header-inner">
      <RouterLink to="/" class="brand" :aria-label="t('UMC Auth home')">
        <img src="/appicon.png" alt="" width="34" height="34" />
        <span class="brand-text">
          <strong>UMC Auth</strong>
          <small>{{ serverName }}</small>
        </span>
      </RouterLink>

      <nav v-if="auth.user" class="main-nav" :aria-label="t('Main')">
        <RouterLink to="/">{{ t('Dashboard') }}</RouterLink>
        <RouterLink to="/sessions">{{ t('Sessions') }}</RouterLink>
        <RouterLink to="/profile">{{ t('Profile') }}</RouterLink>
        <RouterLink v-if="isAdmin" to="/admin">{{ t('Admin') }}</RouterLink>
        <RouterLink v-else-if="isStaff" to="/admin/users">{{ t('Staff') }}</RouterLink>
      </nav>

      <div class="header-actions">
        <LanguageSwitcher />
        <template v-if="auth.user">
          <span class="user-chip" :title="auth.user.email">
            <span class="avatar">{{ initial }}</span>
            <span class="user-name">{{ auth.user.username }}</span>
          </span>
          <button class="btn btn-ghost btn-sm" type="button" @click="signOut">
            {{ t('Sign out') }}
          </button>
        </template>
        <template v-else>
          <RouterLink class="btn btn-ghost btn-sm" to="/login">{{ t('Sign in') }}</RouterLink>
          <RouterLink class="btn btn-primary btn-sm" to="/register">
            {{ t('Create account') }}
          </RouterLink>
        </template>
      </div>
    </div>
  </header>

  <main id="main" :class="minimalLayout ? 'auth-shell' : 'shell main-content'">
    <LanguageSwitcher v-if="minimalLayout" class="auth-language" />
    <div v-if="!auth.ready" class="loading-panel">
      <span class="spinner" aria-hidden="true"></span>
      {{ t('Loading your session…') }}
    </div>
    <RouterView v-else />
  </main>

  <footer v-if="!minimalLayout" class="site-footer">
    <div class="shell">
      {{ t('UMC Auth · community-operated Minecraft account service') }}
    </div>
  </footer>

  <ToastHost />
</template>
