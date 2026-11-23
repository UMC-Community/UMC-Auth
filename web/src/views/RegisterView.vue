<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError, api, registerChallenge } from '../api'
import { setSession } from '../auth'
import { config, loadMeta } from '../config'
import { t } from '../i18n'

const router = useRouter()

const form = reactive({
  username: '',
  email: '',
  password: '',
  confirm: '',
  inviteCode: '',
  bootstrapToken: '',
  turnstileToken: '',
})
const loading = ref(false)
const solving = ref(false)
const error = ref('')
const captchaError = ref('')
const turnstileContainer = ref(null)

const meta = computed(() => config.meta)
const firstAccount = computed(() => Boolean(meta.value?.first_account))
const bootstrapRequired = computed(() => Boolean(meta.value?.bootstrap_required))
const captchaMode = computed(() => meta.value?.captcha_mode || 'none')
const showInvite = computed(
  () => !firstAccount.value && meta.value?.registration_mode === 'invite',
)
const registrationOpen = computed(() => meta.value?.registration_open !== false)

let turnstilePromise = null

function loadTurnstile() {
  if (window.turnstile) return Promise.resolve(window.turnstile)
  if (turnstilePromise) return turnstilePromise
  turnstilePromise = new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    script.async = true
    script.defer = true
    script.onload = () => resolve(window.turnstile)
    script.onerror = () => reject(new Error(t('Could not load Cloudflare Turnstile')))
    document.head.appendChild(script)
  })
  return turnstilePromise
}

function hasLeadingZeroBits(bytes, bits) {
  if (bits <= 0) return true
  const fullBytes = Math.floor(bits / 8)
  for (let index = 0; index < fullBytes; index += 1) {
    if (bytes[index] !== 0) return false
  }
  const remaining = bits % 8
  if (remaining === 0) return true
  const mask = (0xff << (8 - remaining)) & 0xff
  return (bytes[fullBytes] & mask) === 0
}

async function solveProofOfWork(challenge, difficulty) {
  if (!window.crypto?.subtle) {
    throw new Error(t('Proof-of-work requires a secure browser context'))
  }
  const encoder = new TextEncoder()
  for (let nonce = 0; ; nonce += 1) {
    const digest = await window.crypto.subtle.digest(
      'SHA-256',
      encoder.encode(`${challenge}:${nonce}`),
    )
    if (hasLeadingZeroBits(new Uint8Array(digest), difficulty)) {
      return String(nonce)
    }
    if (nonce > 0 && nonce % 250 === 0) {
      await new Promise((resolve) => window.setTimeout(resolve, 0))
    }
  }
}

function validate() {
  if (!/^[A-Za-z0-9_]{3,16}$/.test(form.username)) {
    return t('Username must be 3–16 characters using letters, digits, or underscores.')
  }
  if (form.password.length < 8 || form.password.length > 128) {
    return t('Password must be 8–128 characters.')
  }
  if (form.password !== form.confirm) {
    return t('Passwords do not match.')
  }
  if (!form.email.includes('@')) {
    return t('Enter a valid email address.')
  }
  return ''
}

async function submit() {
  error.value = ''
  const validation = validate()
  if (validation) {
    error.value = validation
    return
  }
  const captcha = captchaMode.value
  if (!firstAccount.value && captcha === 'turnstile' && !form.turnstileToken) {
    error.value = t('Complete the Cloudflare Turnstile challenge.')
    return
  }

  loading.value = true
  try {
    const payload = {
      username: form.username,
      email: form.email,
      password: form.password,
    }
    if (showInvite.value) payload.invite_code = form.inviteCode
    if (bootstrapRequired.value) payload.bootstrap_token = form.bootstrapToken
    if (!firstAccount.value && captcha === 'turnstile') {
      payload.turnstile_token = form.turnstileToken
    }
    if (!firstAccount.value && captcha === 'pow') {
      solving.value = true
      try {
        const challenge = await registerChallenge()
        payload.pow_token = challenge.token
        payload.pow_nonce = await solveProofOfWork(
          challenge.challenge,
          challenge.difficulty,
        )
      } finally {
        solving.value = false
      }
    }

    const data = await api.register(payload)
    if (data?.verification_required) {
      router.push({ name: 'verify-email', query: { email: form.email } })
      return
    }
    setSession(data)
    router.push('/')
  } catch (err) {
    if (err instanceof ApiError && err.code === 'registration_conflict') {
      error.value = t('That username or email is already registered.')
    } else if (err.code === 'invite_required' || err.code === 'invalid_invite') {
      error.value = t('The invite code is missing, invalid, or expired.')
    } else if (err.code === 'bootstrap_required') {
      error.value = t('The bootstrap token is invalid.')
    } else if (err.code === 'captcha_failed') {
      error.value = t('CAPTCHA verification failed. Try again.')
    } else if (err.code === 'registration_closed') {
      error.value = t('Registration is closed on this server.')
    } else if (err.code === 'email_unavailable') {
      error.value = t('Email delivery is not configured. Contact the server operator.')
    } else {
      error.value = err.message
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await loadMeta()
  if (captchaMode.value === 'turnstile' && !firstAccount.value) {
    try {
      const turnstile = await loadTurnstile()
      if (!turnstileContainer.value) return
      if (!meta.value?.turnstile_site_key) {
        captchaError.value = 'Turnstile site key is not configured.'
        return
      }
      turnstile.render(turnstileContainer.value, {
        sitekey: meta.value.turnstile_site_key,
        theme: window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
        callback: (token) => {
          form.turnstileToken = token
        },
        'expired-callback': () => {
          form.turnstileToken = ''
        },
        'error-callback': () => {
          form.turnstileToken = ''
        },
      })
    } catch (err) {
      captchaError.value = err.message
    }
  }
})
</script>

<template>
  <div class="auth-wrap auth-wrap-wide">
    <section class="auth-card">
      <div class="auth-heading">
        <h1>{{ t('Create account') }}</h1>
        <p>{{ t('Enter your details to get started.') }}</p>
      </div>

      <div v-if="config.error" class="alert alert-error">{{ config.error }}</div>
      <div v-if="!registrationOpen" class="alert alert-warning">
        {{ t('Registration is closed. Ask an administrator for an account.') }}
      </div>
      <div v-if="firstAccount" class="alert alert-info">
        {{ t('This is the first account. It becomes the') }}
        <strong>{{ t('superadmin') }}</strong>
        {{ t('and is verified immediately.') }}
      </div>

      <form v-if="registrationOpen" class="form-stack" @submit.prevent="submit">
        <label class="field">
          <span>{{ t('Username') }}</span>
          <input
            v-model.trim="form.username"
            autocomplete="username"
            :placeholder="t('username')"
            minlength="3"
            maxlength="16"
            pattern="[A-Za-z0-9_]+"
            required
            @input="error = ''"
          />
          <small>{{ t('3–16 letters, digits, or underscores.') }}</small>
        </label>

        <label class="field">
          <span>{{ t('Email') }}</span>
          <input
            v-model.trim="form.email"
            type="email"
            autocomplete="email"
            :placeholder="t('name@example.com')"
            required
            @input="error = ''"
          />
          <small v-if="meta?.email_verification === 'required'">
            {{ t('We will send a six-digit verification code.') }}
          </small>
        </label>

        <label class="field">
          <span>{{ t('Password') }}</span>
          <input
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            :placeholder="t('At least 8 characters')"
            minlength="8"
            maxlength="128"
            required
            @input="error = ''"
          />
        </label>

        <label class="field">
          <span>{{ t('Confirm password') }}</span>
          <input
            v-model="form.confirm"
            type="password"
            autocomplete="new-password"
            :placeholder="t('Re-enter your password')"
            required
            @input="error = ''"
          />
        </label>

        <label v-if="showInvite" class="field">
          <span>{{ t('Invite code') }}</span>
          <input
            v-model.trim="form.inviteCode"
            autocomplete="off"
            placeholder="UMC-XXXX-XXXX-XXXX"
            required
          />
        </label>

        <label v-if="bootstrapRequired" class="field">
          <span>{{ t('Bootstrap token') }}</span>
          <input
            v-model="form.bootstrapToken"
            type="password"
            autocomplete="off"
            required
          />
          <small>{{ t('Only needed while the instance has no accounts.') }}</small>
        </label>

        <div v-if="!firstAccount && captchaMode === 'pow'" class="alert alert-info">
          {{ t('This server protects registration with proof-of-work. Your browser solves a short challenge when you submit the form.') }}
        </div>
        <div v-if="!firstAccount && captchaMode === 'turnstile'" class="field">
          <span>{{ t('Human check') }}</span>
          <div ref="turnstileContainer" class="turnstile-slot"></div>
          <small v-if="captchaError" class="field-error">{{ captchaError }}</small>
        </div>

        <button class="btn btn-primary btn-block" type="submit" :disabled="loading || solving">
          {{ loading ? t('Creating account…') : t('Create account') }}
        </button>

        <div v-if="error" class="message error">{{ error }}</div>
        <div v-if="solving" class="message">
          <span class="spinner spinner-sm" aria-hidden="true"></span>
          {{ t('Solving the proof-of-work challenge…') }}
        </div>
      </form>

      <div class="auth-footer">
        {{ t('Already have an account?') }}
        <RouterLink to="/login">{{ t('Sign in') }}</RouterLink>
      </div>
    </section>
  </div>
</template>
