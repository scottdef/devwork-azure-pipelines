# CoolGitOrg repository policy (excerpt used by the harness)

1. Visibility is `private` or `internal`. `public` needs the `approval:public-repo` label.
2. `admin` is granted only to `platform-admins`. Other teams get at most `maintain`.
3. Every repository has a CODEOWNERS owner and the `default-branch` ruleset.
4. Outside collaborators carry an `expires` date no more than 180 days out.
5. Secret scanning push protection is never disabled per repository.
6. Repositories without a push for 18 months are archived or given an owning team.
