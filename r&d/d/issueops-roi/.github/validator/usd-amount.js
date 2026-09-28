import { text } from './lib.js'

/** Whole US dollars, 1..100000 (the registry enforces the self-service maximum). */
export default async (field) => {
  const s = text(field).replace(/^\$/, '').replace(/,/g, '')
  if (!/^\d{1,6}$/.test(s)) return 'Enter a whole number of US dollars, for example `500`.'
  const n = Number(s)
  if (n < 1 || n > 100000) return 'The amount must be between $1 and $100,000.'
  return 'success'
}
