import { LOGIN, REPO, text } from './lib.js'

/** Scope target: blank, a login (@user), a repository ([org/]repo) or a cost-center name. */
export default async (field) => {
  const s = text(field).replace(/^@/, '')
  if (s === '') return 'success'
  if (s.length > 100) return 'The scope target is too long.'
  const [owner, repo] = s.includes('/') ? s.split('/', 2) : [null, s]
  if (owner !== null && !LOGIN.test(owner)) return 'Use `repo`, `org/repo`, a GitHub username or a cost-center name.'
  if (LOGIN.test(repo) || REPO.test(repo) || /^[A-Za-z0-9 ._-]{1,100}$/.test(s)) return 'success'
  return 'Use `repo`, `org/repo`, a GitHub username or a cost-center name.'
}
