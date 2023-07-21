#!/usr/bin/env bash

display_usage() {
	echo -e "Usage: $0 PATH_TO_REPOSITORY_ROOT\n"
}

# Check arguments and set vars with them.
if [ $# -ne 1 ]; then
        display_usage
        exit 1
fi
REPO_ROOT="${1%%/}"

basetime=$(date -d "2023-06-14" +%s)
for f in $(find "${REPO_ROOT}" -type f -path "*/testdata/*"); do
  # Update the file timestamp based on the hash of the contents of the file.
  # This ensures the cache is busted when the contents change while still
  # providing a stable timestamp.
  filehash=$(sha256sum "$f" | tr '[:lower:]' '[:upper:]')
  hashmod10k=$(echo "ibase=16; $(echo $filehash | cut -c1-10) % 2710" | bc)
  modtime=$(( $basetime - $hashmod10k ))
  touch -d "@$modtime" "$f"
  echo "$f timestamp updated to $modtime"
done
