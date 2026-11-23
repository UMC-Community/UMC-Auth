<script setup>
import { languages, locale, setLocale, t } from '../i18n'
</script>

<template>
  <div class="language-switch" role="group" :aria-label="t('Language')">
    <button
      v-for="language in languages"
      :key="language.value"
      type="button"
      :class="{ active: locale === language.value }"
      :aria-pressed="locale === language.value"
      @click="setLocale(language.value)"
    >
      {{ language.label }}
    </button>
  </div>
</template>
