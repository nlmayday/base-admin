#!/usr/bin/env bash
# Turn a copy of this template into a new project:
#   scripts/new-project.sh <go-module-path> "<display name>" [<compose project name>]
# Example:
#   scripts/new-project.sh github.com/acme/crm/server "Acme CRM" acme-crm
#
# It rewrites the Go module path in go.mod and every import, the product
# name shown in the UI, and the docker compose project names. Run it once,
# on a clean copy, then review the diff.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
  exit 2
fi

NEW_MODULE="$1"
NEW_NAME="$2"
NEW_SLUG="${3:-$(printf '%s' "$NEW_NAME" | tr '[:upper:] ' '[:lower:]-' | tr -cd 'a-z0-9-')}"
OLD_MODULE="base-admin/server"
OLD_NAME="Base Admin"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ ! "$NEW_MODULE" =~ ^[A-Za-z0-9._~/-]+$ ]]; then
  echo "invalid Go module path: $NEW_MODULE" >&2
  exit 2
fi
if [[ -z "$NEW_SLUG" ]]; then
  echo "could not derive a compose project name; pass it as the third argument" >&2
  exit 2
fi

cd "$ROOT"
if ! grep -q "^module ${OLD_MODULE}\$" server/go.mod; then
  echo "server/go.mod is not '${OLD_MODULE}' — already renamed?" >&2
  exit 1
fi

# perl -pi works the same on macOS and Linux (unlike sed -i).
export OLD_MODULE NEW_MODULE OLD_NAME NEW_NAME NEW_SLUG

echo "→ Go module path: ${OLD_MODULE} → ${NEW_MODULE}"
# Every text file that names the module: go.mod, Go imports, the code
# generator's templates, the Dockerfile's -ldflags -X, .golangci.yml and docs.
grep -rlI --null --exclude-dir=node_modules --exclude-dir=data --exclude=coverage.out \
  --exclude=go.sum --exclude="$(basename "$0")" -F "$OLD_MODULE" server docs README.md Makefile |
  xargs -0 perl -pi -e 's/(?<![\w.\/-])\Q$ENV{OLD_MODULE}\E(?![\w-])/$ENV{NEW_MODULE}/g'

echo "→ Display name: ${OLD_NAME} → ${NEW_NAME}"
perl -pi -e 's/\Q$ENV{OLD_NAME}\E/$ENV{NEW_NAME}/g' \
  web/public/platform-config.json web/index.html README.md

echo "→ Compose project name: ${NEW_SLUG}"
perl -pi -e 's/^name: base-admin-dev$/name: $ENV{NEW_SLUG}-dev/' deploy/docker/compose.dev.yaml
perl -pi -e 's/^name: base-admin$/name: $ENV{NEW_SLUG}/; s/base-admin-api:latest/$ENV{NEW_SLUG}-api:latest/g' \
  deploy/docker/compose.prod.yaml

echo "→ Verifying the backend still builds"
(cd server && gofmt -l . | tee /dev/stderr | (! read -r) && go build ./... && go vet -tags integration ./...)
if grep -rIl --exclude-dir=node_modules --exclude-dir=data --exclude=coverage.out --exclude=go.sum \
  -F "$OLD_MODULE" server >/dev/null; then
  echo "warning: some files under server/ still mention ${OLD_MODULE}:" >&2
  grep -rIl --exclude-dir=node_modules --exclude-dir=data --exclude=coverage.out --exclude=go.sum \
    -F "$OLD_MODULE" server >&2
fi

cat <<DONE

Done. Next steps:
  1. Review the changes (git diff).
  2. Update README.md and docs/ for your product.
  3. Add your business modules (docs/module-guide.md): write a migration,
     then generate with "go run ./cmd/gen" or 开发工具 → 代码生成.
DONE
