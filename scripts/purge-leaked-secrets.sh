#!/usr/bin/env bash
# =============================================================================
# purge-leaked-secrets.sh — scrub leaked production secrets from git history
# =============================================================================
# Commit 55502b6 added internal/platform/config/http_test.go with production
# values (DATABASE_URL credentials, SESSION_SECRET, SMTP_PASSWORD, bridge
# tokens, Turnstile secret). 7f102cb removed them from the tree but not from
# history. This rewrites every commit so that file's historical versions carry
# "***REMOVED***" instead of the values.
#
# READ FIRST
#   * Purging history does NOT make the secrets safe: anyone who cloned, forked
#     or cached the repository still has them. ROTATE EVERY VALUE FIRST
#     (docs/SECURITY_DATABASE_AND_SECRETS.md, section 5). This script is cleanup.
#   * It rewrites all commit ids on every branch. Everyone must re-clone
#     afterwards; deploy hosts that `git reset --hard origin/main` keep working.
#   * The working tree must be clean (commit or stash first).
#   * Values are read from a local file (default .env) and are never printed or
#     written inside the repository.
#
# USAGE
#   scripts/purge-leaked-secrets.sh [path/to/file-with-old-values.env]
#   then review, and push:  git push --force-with-lease origin --all
#                           git push --force-with-lease origin --tags
# =============================================================================
set -euo pipefail

ENV_FILE="${1:-.env}"
TARGET="internal/platform/config/http_test.go"
KEYS=(SESSION_SECRET SMTP_PASSWORD TELEGRAM_BRIDGE_TOKEN TELEGRAM_BOT_TOKEN
      MARKETING_BRIDGE_TOKEN TURNSTILE_SECRET_KEY GOOGLE_MAPS_API_KEY
      GATEWAY_VIRTUAL_KEY STORAGE_SECRET_ACCESS_KEY)

[ -f "$ENV_FILE" ] || { echo "no $ENV_FILE with the OLD values" >&2; exit 1; }
git diff --quiet && git diff --cached --quiet || { echo "working tree not clean" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
SED="$WORK/scrub.sed"
: > "$SED"

escape() { printf '%s' "$1" | sed -e 's/[]\/$*.^[]/\\&/g'; }
value_of() { grep -E "^$1=" "$ENV_FILE" | tail -n1 | cut -d= -f2- | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'$//"; }

count=0
VALS=()
NAMES=()
for k in "${KEYS[@]}"; do
  v="$(value_of "$k" || true)"
  if [ -n "$v" ] && [ "${#v}" -ge 8 ]; then
    printf 's/%s/***REMOVED***/g\n' "$(escape "$v")" >> "$SED"; count=$((count+1)); VALS+=("$v"); NAMES+=("$k")
  fi
done
# The DATABASE_URL password (between "user:" and "@").
db="$(value_of DATABASE_URL || true)"
dbpw="$(printf '%s' "$db" | sed -nE 's#^[a-z]+://[^:/@]+:([^@]+)@.*#\1#p')"
if [ -n "$dbpw" ]; then printf 's/%s/***REMOVED***/g\n' "$(escape "$dbpw")" >> "$SED"; count=$((count+1)); VALS+=("$dbpw"); NAMES+=("DATABASE_URL password"); fi
echo "scrubbing $count values from every historical version of $TARGET"

export SED TARGET
git filter-branch --force --index-filter '
  blob=$(git ls-files -s -- "$TARGET" | awk "{print \$2}")
  if [ -n "$blob" ]; then
    new=$(git cat-file blob "$blob" | sed -f "$SED" | git hash-object -w --stdin)
    git update-index --cacheinfo 100644,"$new","$TARGET"
  fi
' --tag-name-filter cat -- --all

# Drop the backup refs and unreachable objects so the old blobs are gone locally.
git for-each-ref --format='%(refname)' refs/original/ | xargs -r -n1 git update-ref -d
git reflog expire --expire=now --all
git gc --prune=now --aggressive

# Verify: no value remains in any historical version of $TARGET (fatal), and
# report values that also appear in other files (e.g. a local dev default such
# as minioadmin) so you can judge whether they are real secrets.
left=0
for i in "${!VALS[@]}"; do
  v="${VALS[$i]}"
  if git log --all -p -F -S "$v" --format=%h -- "$TARGET" | grep -q .; then
    echo "!!! ${NAMES[$i]} still present in $TARGET" >&2; left=$((left+1))
  elif git log --all -F -S "$v" --format=%h | grep -q .; then
    echo "note: ${NAMES[$i]}'s local value also appears in other files (not rewritten):" >&2
    git log --all -F -S "$v" --name-only --format= | sort -u | head -5 | sed 's/^/        /' >&2
  fi
done
if [ "$left" -ne 0 ]; then echo "!!! do not push" >&2; exit 1; fi
echo "history clean for $TARGET. Review with 'git log', then force-push (see USAGE)."
