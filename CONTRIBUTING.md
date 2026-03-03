# Contributing

## DCO / Signoff Required

By contributing, you agree to the [Developer Certificate of Origin (DCO)](https://developercertificate.org/).

Every commit must include a `Signed-off-by` trailer that matches the commit author, for example:

```text
Signed-off-by: Jane Doe <jane@example.com>
```

Use Git signoff to add it automatically:

```bash
git commit -s -m "your message"
```

To make signoff automatic for all future commits:

```bash
git config --global format.signoff true
```

If you forgot signoff on the latest commit:

```bash
git commit --amend -s --no-edit
```

## Commit Message Style

Use lightweight Conventional Commits for new commits:

- `feat:` new behavior
- `fix:` bug fix
- `refactor:` internal change with same behavior
- `docs:` documentation-only changes
- `test:` tests-only changes
- `build:` packaging/tooling/dependency changes
- `chore:` maintenance tasks

Preferred format:

```text
type(scope): short imperative summary
```

Example:

```text
fix(release): write stamped image to release/ dir
```
