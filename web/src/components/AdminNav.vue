<script setup>
import { isAdmin } from '../auth'
import { t } from '../i18n'
</script>

<template>
  <nav class="admin-nav" :aria-label="t('Administration')">
    <RouterLink v-if="isAdmin" to="/admin">{{ t('Overview') }}</RouterLink>
    <RouterLink to="/admin/users">{{ t('Users') }}</RouterLink>
    <RouterLink v-if="isAdmin" to="/admin/invites">{{ t('Invites') }}</RouterLink>
    <RouterLink to="/admin/audit">{{ t('Audit') }}</RouterLink>
    <RouterLink v-if="isAdmin" to="/admin/servers">{{ t('Servers') }}</RouterLink>
    <RouterLink v-if="isAdmin" to="/admin/mail">{{ t('Mail') }}</RouterLink>
  </nav>
</template>
