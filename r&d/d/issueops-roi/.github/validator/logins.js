import { LOGIN, list } from './lib.js'

/** Comma-separated GitHub usernames (optional). */
export default async (field) => {
  const bad = list(field)
    .map((l) => l.replace(/^@/, ''))
    .filter((l) => !LOGIN.test(l))
  if (bad.length > 0) return `Not valid GitHub usernames: ${bad.join(', ')}`
  if (list(field).length > 10) return 'At most 10 alert recipients.'
  return 'success'
}
