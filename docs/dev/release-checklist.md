# Release Checklist

Use this checklist for every public release.

## 1) Preflight

1. Work from `release/0.3.0-beta` (or the current release branch).
2. Ensure working tree is clean:
   - `git status`
3. Ensure `flake.lock` is committed (changed or unchanged).

## 2) Version + Changelog

1. Update `version/version.go`:
   - `const Tag = "vX.Y.Z"`
2. Update `CHANGELOG.md` for this release.

## 3) Build Release Artifact

```bash
nix run .#mkRelease
```

Expected artifact:

```text
release/seedetcher-vX.Y.Z.img
```

## 4) Verify Artifact

```bash
sha256sum release/seedetcher-vX.Y.Z.img
# macOS:
shasum -a 256 release/seedetcher-vX.Y.Z.img
```

Optional: boot/smoke test before tagging.

## 5) Third-Party License/Source Check

1. Review `THIRD_PARTY_LICENSES.md`.
2. Update it only if bundled components changed.
3. If local patches were applied to bundled GPL/AGPL components, note patch locations in release notes.

## 6) Tag + Publish

1. Create annotated tag (example):
   - `git tag -a vX.Y.Z -m "vX.Y.Z"`
2. Push branch + tag:
   - `git push`
   - `git push origin vX.Y.Z`
3. Create GitHub release from tag.

## 7) Minimal Release Notes Block

```md
## Source and Licensing
- Source tag: <TAG>
- Third-party licenses: THIRD_PARTY_LICENSES.md
- GPL/AGPL local patches: none
```
