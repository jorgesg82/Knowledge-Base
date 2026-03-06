# Releases

Prebuilt binaries are published as GitHub release assets.

## Assets

Current release assets are named like:

- `kb_<version>_darwin_amd64.tar.gz`
- `kb_<version>_darwin_arm64.tar.gz`
- `kb_<version>_linux_amd64.tar.gz`
- `kb_<version>_linux_arm64.tar.gz`

Each release also includes `checksums.txt`.

## Download Example

```bash
curl -LO https://github.com/jorgesg82/Knowledge-Base/releases/download/v0.2.0/kb_v0.2.0_darwin_arm64.tar.gz
tar xzf kb_v0.2.0_darwin_arm64.tar.gz
```

## Creating a Release

Releases are built automatically by GitHub Actions when you push a tag matching `v*`.

Example:

```bash
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

The release workflow builds the binaries, packages them as `.tar.gz`, generates checksums, and publishes a GitHub release.
