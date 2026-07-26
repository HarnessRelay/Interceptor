#!/bin/sh
set -eu

bin_dir=${HARNESSRELAY_BIN_DIR:-"${HOME:?HOME is required}/.local/bin"}
config_home=${XDG_CONFIG_HOME:-"$HOME/.config"}
data_home=${XDG_DATA_HOME:-"$HOME/.local/share"}
state_home=${XDG_STATE_HOME:-"$HOME/.local/state"}
config_dir="$config_home/harnessrelay"
data_dir="$data_home/harnessrelay"
state_dir="$state_home/harnessrelay"
manifest="$config_dir/install-manifest"
purge=0
remove_shims=0
remaining=0

usage() {
	printf '%s\n' "Usage: scripts/uninstall.sh [--shims] [--purge]"
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--shims) remove_shims=1 ;;
		--purge) purge=1; remove_shims=1 ;;
		--help|-h) usage; exit 0 ;;
		*) usage >&2; printf 'uninstall: unknown option: %s\n' "$1" >&2; exit 2 ;;
	esac
	shift
done

hash_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		printf '%s\n' "uninstall: sha256sum or shasum is required" >&2
		exit 1
	fi
}

manifest_value() {
	key=$1
	[ -f "$manifest" ] || return 1
	awk -F= -v key="$key" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$manifest"
}

if [ ! -f "$manifest" ]; then
	printf 'uninstall: install manifest not found: %s\n' "$manifest" >&2
	printf '%s\n' "No binaries were removed." >&2
	exit 1
fi

if [ "$remove_shims" -eq 1 ] && [ -x "$bin_dir/harnessctl" ]; then
	"$bin_dir/harnessctl" shims uninstall-all || {
		printf '%s\n' "uninstall: shims could not be removed; binaries were left in place" >&2
		exit 1
	}
fi

printf '%s\n' "Removed:"
for name in harnessctl harnessd; do
	path=$(manifest_value "${name}_path" || true)
	recorded_hash=$(manifest_value "${name}_sha256" || true)
	expected="$bin_dir/$name"
	if [ "$path" != "$expected" ] || [ -z "$recorded_hash" ]; then
		printf 'uninstall: invalid ownership record for %s; left in place\n' "$expected" >&2
		remaining=1
		continue
	fi
	if [ ! -e "$path" ]; then
		continue
	fi
	current_hash=$(hash_file "$path")
	if [ "$current_hash" != "$recorded_hash" ]; then
		printf 'uninstall: modified or unmanaged file left in place: %s\n' "$path" >&2
		remaining=1
		continue
	fi
	rm -f -- "$path"
	printf '  %s\n' "$path"
done

if [ "$remaining" -eq 1 ]; then
	printf '%s\n' "The install manifest and user data were preserved for recovery." >&2
	exit 1
fi

rm -f -- "$manifest"

if [ "$purge" -eq 1 ]; then
	for dir in "$config_dir" "$data_dir" "$state_dir"; do
		case "$dir" in
			*/harnessrelay) rm -rf -- "$dir" ;;
			*) printf 'uninstall: refusing unsafe purge path: %s\n' "$dir" >&2; exit 1 ;;
		esac
	done
	printf '%s\n' "Purged HarnessRelay config, data, state, and owned shims."
else
	printf '\n%s\n' "Left in place:"
	printf '  %s\n  %s\n  %s\n' "$config_dir" "$data_dir" "$state_dir"
	printf '\n%s\n' "To remove owned shims too: scripts/uninstall.sh --shims"
	printf '%s\n' "To remove all HarnessRelay user data: scripts/uninstall.sh --purge"
fi
