<script setup>
import { computed } from 'vue'
import { auth, isAdmin, isStaff } from '../auth'
import { config } from '../config'
import { copyText, formatDate, prettyUuid } from '../format'
import { t } from '../i18n'
import { notify } from '../toast'
import StatusBadge from '../components/StatusBadge.vue'

const user = computed(() => auth.user)
const yggdrasilUrl = computed(
  () => config.meta?.yggdrasil_url || `${config.meta?.auth_origin || ''}/api/yggdrasil`,
)
const roleCopy = computed(() => {
  const role = user.value?.role || 'user'
  if (role === 'superadmin') return t('Superadmin')
  if (role === 'admin') return t('Administrator')
  if (role === 'moderator') return t('Moderator')
  return t('Player')
})

async function copy(value, label) {
  if (!value) return
  try {
    await copyText(value)
    notify(t('{label} copied', { label: t(label) }), 'success', 2200)
  } catch {
    notify(t('Could not copy to the clipboard'), 'error')
  }
}
</script>

<template>
  <div class="page-stack" v-if="user">
    <section class="hero-panel">
      <div class="hero-copy">
        <p class="eyebrow">{{ t('Your account') }}</p>
        <h1>{{ t('Welcome back, {name}', { name: user.username }) }}</h1>
        <p class="muted">
          {{ t('This account is used for the website, your launcher, and the Minecraft server.') }}
        </p>
      </div>
      <div class="hero-badges">
        <StatusBadge :value="user.role" kind="role" />
        <StatusBadge :value="user.status" kind="status" />
        <span v-if="user.email_verified" class="badge badge-verified">
          {{ t('email verified') }}
        </span>
        <span v-else class="badge badge-warning">{{ t('email not verified') }}</span>
      </div>
    </section>

    <div class="card-grid">
      <section class="card">
        <h2>{{ t('Account') }}</h2>
        <dl class="detail-list">
          <div><dt>{{ t('Username') }}</dt><dd>{{ user.username }}</dd></div>
          <div><dt>{{ t('Email') }}</dt><dd>{{ user.email || '—' }}</dd></div>
          <div><dt>{{ t('Role') }}</dt><dd>{{ roleCopy }}</dd></div>
          <div><dt>{{ t('Created') }}</dt><dd>{{ formatDate(user.created_at) }}</dd></div>
        </dl>
        <div class="card-actions">
          <RouterLink class="btn btn-secondary btn-sm" to="/profile">
            {{ t('View profile') }}
          </RouterLink>
          <RouterLink class="btn btn-ghost btn-sm" to="/sessions">
            {{ t('Manage sessions') }}
          </RouterLink>
        </div>
      </section>

      <section class="card">
        <h2>{{ t('Minecraft identity') }}</h2>
        <p class="muted">
          {{ t('Use this UUID when adding the player to a server whitelist or admin list.') }}
        </p>
        <div class="copy-row">
          <code>{{ prettyUuid(user.uuid) }}</code>
          <button class="btn btn-ghost btn-sm" type="button" @click="copy(user.uuid, 'UUID')">
            {{ t('Copy') }}
          </button>
        </div>
        <div class="copy-row">
          <code class="code-muted">{{ user.uuid }}</code>
          <button class="btn btn-ghost btn-sm" type="button" @click="copy(user.uuid, 'UUID')">
            {{ t('Copy raw') }}
          </button>
        </div>
      </section>

      <section class="card">
        <h2>{{ t('Launcher endpoint') }}</h2>
        <p class="muted">
          {{ t('Give this Yggdrasil URL to authlib-injector or a launcher that supports external authentication.') }}
        </p>
        <div class="copy-row">
          <code class="code-wrap">{{ yggdrasilUrl }}</code>
          <button
            class="btn btn-ghost btn-sm"
            type="button"
            @click="copy(yggdrasilUrl, 'Yggdrasil URL')"
          >
            {{ t('Copy') }}
          </button>
        </div>
      </section>

      <section v-if="isStaff" class="card card-accent">
        <h2>{{ t(isAdmin ? 'Administration' : 'Staff tools') }}</h2>
        <p class="muted">
          {{
            t(
              isAdmin
                ? 'Manage accounts, roles, audit records, and the mail outbox.'
                : 'Review accounts and the audit log.',
            )
          }}
        </p>
        <div class="card-actions">
          <RouterLink v-if="isAdmin" class="btn btn-primary btn-sm" to="/admin">
            {{ t('Open admin') }}
          </RouterLink>
          <RouterLink class="btn btn-secondary btn-sm" to="/admin/users">
            {{ t('Users') }}
          </RouterLink>
          <RouterLink class="btn btn-ghost btn-sm" to="/admin/audit">
            {{ t('Audit log') }}
          </RouterLink>
        </div>
      </section>
    </div>
  </div>
</template>
