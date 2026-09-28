import { text } from './lib.js'

/** CoolEngEnt chargeback cost centers look like CC-4410 or CC-4410-PAYMENTS. */
export default async (field) => {
  const s = text(field)
  if (!/^CC-\d{4}(-[A-Z0-9]{2,20})?$/.test(s)) return 'Use a cost center such as `CC-4410` or `CC-4410-PAYMENTS`.'
  return 'success'
}
