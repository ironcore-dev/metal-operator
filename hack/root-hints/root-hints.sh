#!/usr/bin/env bash

set -eu

function err() {
	echo "$@" 1>&2
}

function filter() {
	col=$(echo "$1" | cut -d: -f1)
	f=$(echo "$1" | cut -d: -f2- | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' )
	r="([a-zA-Z0-9-]+) ([a-zA-Z0-9-]+)"
	if [[ $f =~ $r ]]; then
		op="${BASH_REMATCH[1]}"
		val="${BASH_REMATCH[2]}"
	else
		op="eq"
		val="$f"
	fi
	if [ "$op" == "re" ]; then
		op="=~"
	fi
	if [[ $val =~ ^[0-9]+[KMGT]+$ ]]; then
		val=$(echo "$val" | numfmt --from=iec)
		echo "and ${col^^} $op ${val}"
	else
		echo "and ${col^^} $op \"${val}\""
	fi

}

yamlFile="$1"

# validate yaml

#if ! yq . "$yamlFile" &> /dev/null; then
#	err "invalid yaml file provided, exiting"
#	exit 1
#fi

#if dev=$(yq -e .dev "$yamlFile" 2> /dev/null); then
#	echo "$dev"
#	exit 0
#fi

flsblk="TYPE eq \"disk\""
while IFS= read -r r; do
	f=$(filter "$r")
	flsblk="$flsblk $f"
done < <(cat "$yamlFile")

#echo "$flsblk"
lsblk --filter "$flsblk" --noheadings -o kname
