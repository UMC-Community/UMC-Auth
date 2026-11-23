<script setup>
import { computed, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '../api'
import { setSession } from '../auth'
import { t } from '../i18n'

const route = useRoute()
const router = useRouter()

const form = reactive({
  login: typeof route.query.email === 'string' ? route.query.email : '',
  password: '',
})
const loading = ref(false)
const error = ref('')
const errorCode = ref('')
const showPassword = ref(false)

const notice = computed(() => {
  if (route.query.verified === '1') return t('Email verified. You can sign in now.')
  if (route.query.reset === '1') return t('Password updated. Sign in with your new password.')
  return ''
})

async function submit() {
  error.value = ''
  errorCode.value = ''
  loading.value = true
  try {
    const data = await api.login(form.login, form.password)
    setSession(data)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    router.push(redirect.startsWith('/') ? redirect : '/')
  } catch (err) {
    error.value = err.message
    errorCode.value = err.code || ''
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-wrap">
    <section class="auth-card">
      <div class="auth-heading">
        <h1>{{ t('Welcome back') }}</h1>
        <p>{{ t('Sign in with your username or email to continue.') }}</p>
      </div>

      <div v-if="notice" class="alert alert-success">{{ notice }}</div>

      <form class="form-stack" @submit.prevent="submit">
        <label class="field">
          <span>{{ t('Email or username') }}</span>
          <input
            v-model.trim="form.login"
            autocomplete="username"
            :placeholder="t('name@example.com')"
            autofocus
            required
            @input="error = ''"
          />
        </label>

        <label class="field">
          <span>{{ t('Password') }}</span>
          <span class="input-with-action">
            <input
              v-model="form.password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="••••••••"
              required
              @input="error = ''"
            />
            <button
              type="button"
              class="input-action"
              @click="showPassword = !showPassword"
            >
              {{ showPassword ? t('Hide') : t('Show') }}
            </button>
          </span>
        </label>

        <div class="auth-helper">
          <RouterLink to="/forgot-password">{{ t('Forgot password?') }}</RouterLink>
        </div>

        <button class="btn btn-primary btn-block" type="submit" :disabled="loading">
          {{ loading ? t('Signing in…') : t('Sign in') }}
        </button>

        <div v-if="error" class="message error">
          {{ error }}
          <RouterLink
            v-if="errorCode === 'email_not_verified'"
            :to="{ name: 'verify-email', query: { email: form.login } }"
          >
            {{ t('Verify email') }}
          </RouterLink>
        </div>
      </form>

      <div class="auth-footer">
        {{ t("Don't have an account?") }}
        <RouterLink to="/register">{{ t('Create one') }}</RouterLink>
      </div>
    </section>
  </div>
</template>
