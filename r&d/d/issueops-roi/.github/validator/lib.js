// Shared helpers for the custom validators (ESM, Node built-ins only).

/** Normalizes an issue-ops/parser value to a trimmed string ('' when empty). */
export function text(field) {
  if (field === undefined || field === null) return ''
  if (Array.isArray(field)) return field.join(', ').trim()
  if (typeof field === 'object') return ''
  const s = String(field).trim()
  return s === '_No response_' ? '' : s
}

/** Splits a comma/newline separated list. */
export function list(field) {
  return text(field)
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

export const LOGIN = /^[A-Za-z0-9](?:[A-Za-z0-9]|-(?=[A-Za-z0-9])){0,38}$/
export const REPO = /^[A-Za-z0-9._-]{1,100}$/
export const TEAM_SLUG = /^[a-z0-9][a-z0-9_-]{0,99}$/

const SECRET =
  /(ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|gh[ousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|xox[baprs]-[A-Za-z0-9-]{10,}|AccountKey=[A-Za-z0-9+/=]{20,}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})/i

export function looksLikeSecret(s) {
  return SECRET.test(s)
}

/**
 * Calls the GitHub REST API with the IssueOps app token passed by the workflow
 * (ISSUEOPS_VALIDATOR_TOKEN). Returns the HTTP status, or 0 when no token is
 * configured (local runs): callers then skip existence checks.
 */
export async function ghStatus(path) {
  const token = process.env.ISSUEOPS_VALIDATOR_TOKEN
  if (!token) return 0
  const base = (process.env.GITHUB_API_URL || 'https://api.github.com').replace(/\/$/, '')
  const res = await fetch(`${base}/${path}`, {
    headers: {
      Accept: 'application/vnd.github+json',
      Authorization: `Bearer ${token}`,
      'X-GitHub-Api-Version': '2022-11-28'
    }
  })
  return res.status
}

export function org() {
  return process.env.ORGANIZATION || 'CoolEngOrg'
}
