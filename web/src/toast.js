import { reactive } from 'vue'

export const toasts = reactive({ items: [] })

let nextId = 1

export function notify(message, type = 'info', timeout = 4200) {
  const id = nextId++
  toasts.items.push({ id, message, type })
  if (timeout > 0) {
    window.setTimeout(() => dismiss(id), timeout)
  }
  return id
}

export function dismiss(id) {
  const index = toasts.items.findIndex((item) => item.id === id)
  if (index >= 0) toasts.items.splice(index, 1)
}
