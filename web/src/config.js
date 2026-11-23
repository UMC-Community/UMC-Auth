import { reactive } from 'vue'
import { api } from './api'
import { t } from './i18n'

export const config = reactive({
  meta: null,
  loaded: false,
  error: '',
})

export async function loadMeta() {
  if (config.loaded) return config.meta
  try {
    config.meta = await api.meta()
    config.error = ''
    return config.meta
  } catch (error) {
    config.error = error.message || t('Could not load server metadata')
    return null
  } finally {
    config.loaded = true
  }
}
