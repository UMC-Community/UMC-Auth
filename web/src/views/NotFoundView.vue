<script setup>
import { t } from '../i18n'
</script>

<template>
  <div class="auth-wrap">
    <section class="auth-card auth-card-centered">
      <p class="eyebrow">404</p>
      <h1>{{ t('Page not found') }}</h1>
      <p class="muted">{{ t('That page does not exist or you do not have access to it.') }}</p>
      <RouterLink class="btn btn-primary" to="/">{{ t('Return to dashboard') }}</RouterLink>
    </section>
  </div>
</template>
