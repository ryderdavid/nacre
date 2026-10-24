#!/bin/sh
# Bumps the app's version once per round (nacre.yaml version.bump).
#   scripts/bump-version.sh patch   0.3.3 (7) -> 0.3.4 (8)
#   scripts/bump-version.sh minor   0.3.3 (7) -> 0.4.0 (8)
# versionCode always goes up by one. Prints the new version.
set -eu
level="${1:-}"
case "$level" in patch|minor) ;; *) echo "usage: $0 patch|minor" >&2; exit 2;; esac
file="$(cd "$(dirname "$0")/.." && pwd)/android/app/build.gradle.kts"

code=$(sed -n 's/^ *versionCode = \([0-9][0-9]*\).*/\1/p' "$file")
name=$(sed -n 's/^ *versionName = "\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)".*/\1/p' "$file")
if [ -z "$code" ] || [ -z "$name" ]; then
  echo "can't find versionCode and a major.minor.patch versionName in $file" >&2
  exit 1
fi
major=${name%%.*}; rest=${name#*.}; minor=${rest%%.*}; patch=${rest#*.}
if [ "$level" = minor ]; then minor=$((minor + 1)); patch=0; else patch=$((patch + 1)); fi
new="$major.$minor.$patch"
newcode=$((code + 1))

tmp="$file.tmp.$$"
sed -e "s/^\( *versionCode = \)$code/\1$newcode/" \
    -e "s/^\( *versionName = \"\)$name\"/\1$new\"/" "$file" > "$tmp"
mv "$tmp" "$file"
echo "$name ($code) -> $new ($newcode)"
