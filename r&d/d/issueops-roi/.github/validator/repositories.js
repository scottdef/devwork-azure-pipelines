import { REPO, ghStatus, list, org } from './lib.js'

/** Comma-separated repository names ([org/]repo); each must exist in the organization. */
export default async (field) => {
  const names = []
  for (const r of list(field)) {
    const parts = r.split('/')
    if (parts.length === 2 && parts[0].toLowerCase() !== org().toLowerCase()) return `\`${r}\` is outside ${org()}.`
    const name = parts[parts.length - 1]
    if (parts.length > 2 || !REPO.test(name)) return `\`${r}\` is not a valid repository name.`
    names.push(name)
  }
  if (names.length > 25) return 'At most 25 repositories.'
  for (const n of names) {
    const status = await ghStatus(`repos/${encodeURIComponent(org())}/${encodeURIComponent(n)}`)
    if (status === 404) return `Repository \`${org()}/${n}\` was not found (or the IssueOps app cannot see it).`
    if (status >= 400) return `Could not verify repository \`${n}\` (HTTP ${status}).`
  }
  return 'success'
}
