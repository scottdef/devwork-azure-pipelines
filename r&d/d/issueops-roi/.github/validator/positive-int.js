import { text } from './lib.js'

/** Positive whole number (capacity in thousands of TPM or PTUs). */
export default async (field) => {
  const s = text(field).replace(/,/g, '')
  if (!/^\d{1,6}$/.test(s) || Number(s) < 1) return 'Enter a positive whole number.'
  return 'success'
}
