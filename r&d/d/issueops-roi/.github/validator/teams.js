import { TEAM_SLUG, ghStatus, list, org } from './lib.js'

/** Comma-separated team slugs (optionally @org/slug); each must exist in the organization. */
export default async (field) => {
  const o = org().toLowerCase()
  const slugs = list(field).map((t) => t.replace(/^@/, '').toLowerCase().replace(`${o}/`, ''))
  const bad = slugs.filter((s) => !TEAM_SLUG.test(s))
  if (bad.length > 0) return `Not valid team slugs: ${bad.join(', ')}`
  if (slugs.length > 20) return 'At most 20 teams per report.'
  for (const s of slugs) {
    const status = await ghStatus(`orgs/${encodeURIComponent(org())}/teams/${encodeURIComponent(s)}`)
    if (status === 404) return `Team \`${s}\` was not found in ${org()}.`
    if (status >= 400) return `Could not verify team \`${s}\` (HTTP ${status}).`
  }
  return 'success'
}
