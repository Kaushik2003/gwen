#!/usr/bin/env bash
# Enforces the docs/ change protocol mechanically.
# What it checks is specified in docs/README.md#what-make-docs-check-verifies.
set -euo pipefail

cd "$(dirname "$0")/.."

readonly DOCS=docs
readonly MAX_LINES=500
readonly MANIFEST=(
  README.md
  CONVENTIONS.md
  01-product.md
  02-architecture.md
  03-data-model.md
  04-api-contract.md
  05-time-engine.md
  06-planner.md
  07-integrations.md
  08-clients.md
  09-packaging.md
  10-build-plan.md
  11-testing.md
)

failed=0
err() {
  printf 'docs-check: %s\n' "$*" >&2
  failed=1
}

# 1. The manifest is closed: nothing extra, nothing missing.
expected=$(printf '%s\n' "${MANIFEST[@]}" | LC_ALL=C sort)
actual=$(cd "$DOCS" && find . -mindepth 1 -printf '%P\n' | LC_ALL=C sort)
while IFS= read -r extra; do
  [[ -n $extra ]] && err "$DOCS/$extra: not in the manifest (the manifest is closed)"
done < <(LC_ALL=C comm -13 <(printf '%s\n' "$expected") <(printf '%s\n' "$actual"))
while IFS= read -r missing; do
  [[ -n $missing ]] && err "$DOCS/$missing: listed in the manifest but missing"
done < <(LC_ALL=C comm -23 <(printf '%s\n' "$expected") <(printf '%s\n' "$actual"))

files=()
for name in "${MANIFEST[@]}"; do
  [[ -f $DOCS/$name ]] && files+=("$DOCS/$name")
done

# 2. Line budget.
for f in "${files[@]}"; do
  lines=$(wc -l < "$f")
  if (( lines > MAX_LINES )); then
    err "$f: $lines lines, over the $MAX_LINES-line budget"
  fi
done

# 3-5. One pass per file, outside fenced code blocks, emitting:
#   ANCHOR <file> <slug>          every heading, slugged the way GitHub does
#   BANNED <file> <line> <text>   changelog/history/deprecated headings
#   MARKER <file> <line>          TBD, TODO, FIXME or ??? outside inline code
#   LINK   <file> <line> <target> every markdown link outside inline code
scan='
function slug(s) {
  s = tolower(s)
  gsub(/[^a-z0-9 _-]/, "", s)
  gsub(/ /, "-", s)
  return s
}
FNR == 1 { fence = 0; delete seen }
/^[[:space:]]*(```|~~~)/ { fence = !fence; next }
fence { next }
{
  line = $0
  if (match(line, /^#+[[:space:]]+/)) {
    text = substr(line, RLENGTH + 1)
    sub(/[[:space:]]+#+[[:space:]]*$/, "", text)
    if (tolower(text) ~ /^(changelog|history|deprecated)([^a-z]|$)/)
      print "BANNED\t" FILENAME "\t" FNR "\t" text
    s = slug(text)
    n = seen[s]++
    if (n > 0) s = s "-" n
    print "ANCHOR\t" FILENAME "\t" s
  }
  bare = line
  gsub(/`[^`]*`/, "", bare)
  if (bare ~ /(^|[^A-Za-z])(TBD|TODO|FIXME)([^A-Za-z]|$)/ || index(bare, "???"))
    print "MARKER\t" FILENAME "\t" FNR
  while (match(bare, /\]\([^)]*\)/)) {
    print "LINK\t" FILENAME "\t" FNR "\t" substr(bare, RSTART + 2, RLENGTH - 3)
    bare = substr(bare, RSTART + RLENGTH)
  }
}
'

index_file=$(mktemp)
trap 'rm -f "$index_file"' EXIT
awk "$scan" "${files[@]}" > "$index_file"

while IFS=$'\t' read -r kind file lineno detail; do
  case $kind in
    BANNED)
      err "$file:$lineno: banned heading \"$detail\" (git is the changelog)"
      ;;
    MARKER)
      err "$file:$lineno: unresolved-decision marker; resolve the decision in the spec"
      ;;
    LINK)
      target=$detail
      [[ $target =~ ^[a-zA-Z][a-zA-Z0-9+.-]*: ]] && continue # http:, https:, mailto:
      path=${target%%#*}
      anchor=
      [[ $target == *'#'* ]] && anchor=${target#*#}
      if [[ -z $path ]]; then
        resolved=$file
      else
        resolved=$(realpath -m --relative-to=. "$(dirname "$file")/$path")
      fi
      if [[ ! -e $resolved ]]; then
        err "$file:$lineno: link target \"$target\" does not exist"
        continue
      fi
      if [[ -n $anchor ]] && ! grep -qxF "ANCHOR"$'\t'"$resolved"$'\t'"$anchor" "$index_file"; then
        err "$file:$lineno: anchor \"#$anchor\" not found in $resolved"
      fi
      ;;
  esac
done < "$index_file"

if (( failed )); then
  exit 1
fi
printf 'docs-check: ok (%d files)\n' "${#files[@]}"
