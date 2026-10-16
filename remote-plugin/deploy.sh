#!/bin/bash
# Build fsbhoa-remote-lighting.zip for upload to the HOA (Bluehost) WordPress site.
#
# The zip must contain a top-level "fsbhoa-remote-lighting/" folder matching the
# installed plugin's folder name. Otherwise WordPress installs it as a second,
# separate plugin instead of offering "Replace current with uploaded".
set -e
cd "$(dirname "$0")"

PLUGIN_SLUG="fsbhoa-remote-lighting"
ZIP="$PWD/$PLUGIN_SLUG.zip"
rm -f "$ZIP"

STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
mkdir "$STAGE/$PLUGIN_SLUG"
cp -r "$PLUGIN_SLUG.php" assets "$STAGE/$PLUGIN_SLUG/"
(cd "$STAGE" && zip -r -q "$ZIP" "$PLUGIN_SLUG")

unzip -l "$ZIP"
echo "On the PC use cmd:"
echo "scp fsbhoa@access.fsbhoa.com:~/fsbhoa_light/remote-plugin/fsbhoa-remote-lighting.zip %USERPROFILE%/Downloads/"
