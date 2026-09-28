import { text } from './lib.js'

/** Optional calendar date in YYYY-MM-DD. */
export default async (field) => {
  const s = text(field)
  if (s === '') return 'success'
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return 'Use the YYYY-MM-DD format.'
  const d = new Date(`${s}T00:00:00Z`)
  if (Number.isNaN(d.getTime()) || d.toISOString().slice(0, 10) !== s) return `${s} is not a real calendar date.`
  return 'success'
}
