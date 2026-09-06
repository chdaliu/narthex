#!/usr/bin/env bash
# Download open-source icons and background images into web/assets.
#
# Icons : Lucide (ISC license)  https://lucide.dev  (lucide-static@0.469.0)
# Photos: Lorem Picsum, photos from Unsplash (free to use)
#         https://picsum.photos  https://unsplash.com/license
#
# The downloaded files are committed to the repository so that builds and
# deployments work offline. Re-run this script only to refresh assets.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ICON_DIR="$ROOT/web/assets/icons"
BG_DIR="$ROOT/web/assets/backgrounds"
ICON_VERSION="0.469.0"

mkdir -p "$ICON_DIR" "$BG_DIR"

ICONS=(
  folder folder-open code braces terminal rocket database globe bot sparkles
  git-branch git-merge book book-open package layers cpu server cloud flame
  star zap hammer wrench palette camera music boxes pencil trash-2 search
  plus x external-link power menu download
)

BG_IDS=(1015 1018 1035 1036 1039 1043 1050 1053)
BG_FALLBACK_IDS=(10 11 28 29 37 48 60 110)
# 2560x1440 keeps photos crisp on retina displays.
BG_SIZE="2560/1440"

download_icon() {
  local name="$1"
  local out="$ICON_DIR/$name.svg"
  if [ -s "$out" ]; then
    echo "skip   icon/$name.svg (exists)"
    return 0
  fi
  echo "fetch  icon/$name.svg"
  curl -fsSL --retry 3 -o "$out" \
    "https://unpkg.com/lucide-static@${ICON_VERSION}/icons/${name}.svg"
  if ! grep -q '<svg' "$out"; then
    rm -f "$out"
    echo "error  icon/$name.svg is not an SVG" >&2
    return 1
  fi
}

is_jpeg() {
  local f="$1"
  [ -s "$f" ] && [ "$(head -c 2 "$f" | od -An -tx1 | tr -d ' \n')" = "ffd8" ]
}

download_bg() {
  local idx="$1"
  local id="$2"
  local out="$BG_DIR/bg-$(printf '%02d' "$idx").jpg"
  if [ -s "$out" ] && is_jpeg "$out"; then
    echo "skip   backgrounds/bg-$(printf '%02d' "$idx").jpg (exists)"
    return 0
  fi
  echo "fetch  backgrounds/bg-$(printf '%02d' "$idx").jpg (picsum id $id)"
  if ! curl -fsSL --retry 3 -o "$out" "https://picsum.photos/id/$id/$BG_SIZE.jpg"; then
    echo "warn   picsum id $id failed, trying fallback" >&2
    local fallback="${BG_FALLBACK_IDS[$((idx - 1))]}"
    curl -fsSL --retry 3 -o "$out" "https://picsum.photos/id/$fallback/$BG_SIZE.jpg"
  fi
  if ! is_jpeg "$out"; then
    rm -f "$out"
    echo "error  backgrounds/bg-$(printf '%02d' "$idx").jpg is not a JPEG" >&2
    return 1
  fi
}

for name in "${ICONS[@]}"; do
  download_icon "$name"
done

i=1
for id in "${BG_IDS[@]}"; do
  download_bg "$i" "$id"
  i=$((i + 1))
done

echo "done: $(ls "$ICON_DIR" | wc -l | tr -d ' ') icons, $(ls "$BG_DIR" | wc -l | tr -d ' ') backgrounds"
