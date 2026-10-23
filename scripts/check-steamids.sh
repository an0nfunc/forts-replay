#!/usr/bin/env bash
# Fails when a text file holds an individual SteamID64, one whose upper 32
# bits are 0x01100001, that scripts/steamids.allow does not list: as 17
# decimal digits, as 0x011000..., or as little-endian bytes in hex, spaced or
# not, ending in 01 00 10 01. Checks the given files, or else every tracked
# file outside the corpus submodule.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
allow=$(grep -Eo '^[0-9]+' "$root/scripts/steamids.allow" || true)

files=("$@")
if [ ${#files[@]} -eq 0 ]; then
	cd "$root"
	mapfile -d '' files < <(git ls-files -z) # grep -d skip passes over the submodule
fi

pattern='(?<![0-9])[0-9]{17}(?![0-9])|0x011000[0-9a-f]{10}|([0-9a-f]{2} ){4}01 00 10 01|[0-9a-f]{8}01001001'
found=0
while IFS= read -r hit; do
	m=${hit#*:*:}
	if [[ $m =~ ^[0-9]{17}$ ]]; then
		id=$((10#$m))
	elif [[ $m == 0[xX]* ]]; then
		id=$((m))
	else
		h=${m// /}
		id=$((16#${h:14:2}${h:12:2}${h:10:2}${h:8:2}${h:6:2}${h:4:2}${h:2:2}${h:0:2}))
	fi
	if ((id >> 32 == 0x01100001)) && ! grep -qx "$id" <<<"$allow"; then
		echo "${hit%"$m"}$id: SteamID64 not in scripts/steamids.allow" >&2
		found=1
	fi
done < <(grep -IHnoiP -d skip -- "$pattern" "${files[@]}" || true)
exit "$found"
