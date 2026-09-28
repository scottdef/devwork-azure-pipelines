import { looksLikeSecret, text } from './lib.js'

/** Refuses text that looks like a credential (it would be copied into prompts and comments). */
export default async (field) => {
  if (looksLikeSecret(text(field))) return 'This looks like a credential or secret. Remove it from the issue (and rotate it).'
  return 'success'
}
