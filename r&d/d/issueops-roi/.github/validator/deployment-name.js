import { text } from './lib.js'

/** Optional Foundry deployment name (the CLI derives one when blank). */
export default async (field) => {
  const s = text(field)
  if (s === '') return 'success'
  if (!/^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$/.test(s)) return 'Use 2-64 letters, digits, `.`, `_` or `-`, starting with a letter or digit.'
  return 'success'
}
