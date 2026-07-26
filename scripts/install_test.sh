#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT HUP INT TERM

fail() {
	printf 'install_test: %s\n' "$1" >&2
	exit 1
}

make_fixture() {
	fixture_dir=$1
	version=$2
	mkdir -p "$fixture_dir"
	for name in harnessctl harnessd; do
		{
			printf '%s\n' '#!/bin/sh'
			printf 'printf "fixture %s %s\\n"\n' "$name" "$version"
		} >"$fixture_dir/$name"
		chmod 755 "$fixture_dir/$name"
	done
}

run_install() {
	home_dir=$1
	source_dir=$2
	shift 2
	env \
		HOME="$home_dir" \
		XDG_CONFIG_HOME="$home_dir/.config" \
		XDG_DATA_HOME="$home_dir/.local/share" \
		XDG_STATE_HOME="$home_dir/.local/state" \
		HARNESSRELAY_SOURCE_BIN_DIR="$source_dir" \
		PATH="/usr/bin:/bin" \
		"$script_dir/install.sh" --skip-build "$@"
}

run_uninstall() {
	home_dir=$1
	shift
	env \
		HOME="$home_dir" \
		XDG_CONFIG_HOME="$home_dir/.config" \
		XDG_DATA_HOME="$home_dir/.local/share" \
		XDG_STATE_HOME="$home_dir/.local/state" \
		PATH="/usr/bin:/bin" \
		"$script_dir/uninstall.sh" "$@"
}

source_dir="$test_root/source"
home_dir="$test_root/home"
make_fixture "$source_dir" v1
output=$(run_install "$home_dir" "$source_dir")

[ -x "$home_dir/.local/bin/harnessctl" ] || fail "harnessctl was not installed"
[ -x "$home_dir/.local/bin/harnessd" ] || fail "harnessd was not installed"
[ -d "$home_dir/.config/harnessrelay" ] || fail "config directory was not created"
[ -d "$home_dir/.local/share/harnessrelay" ] || fail "data directory was not created"
[ -f "$home_dir/.config/harnessrelay/token" ] || fail "stable token was not created"
[ "$(stat -c %a "$home_dir/.config/harnessrelay")" = 700 ] || fail "config directory mode is not 0700"
[ "$(stat -c %a "$home_dir/.config/harnessrelay/token")" = 600 ] || fail "token mode is not 0600"
printf '%s' "$output" | grep -q "is not in PATH" || fail "missing PATH warning"
printf '%s' "$output" | grep -q "No shell profile or harness shim was changed" || fail "missing no-profile-edit notice"

token_before=$(cat "$home_dir/.config/harnessrelay/token")
printf '%s\n' '# preserved user setting' >>"$home_dir/.config/harnessrelay/interceptor.toml"
make_fixture "$source_dir" v2
run_install "$home_dir" "$source_dir" --update >/dev/null
token_after=$(cat "$home_dir/.config/harnessrelay/token")
[ "$token_before" = "$token_after" ] || fail "update replaced the stable token"
grep -q "preserved user setting" "$home_dir/.config/harnessrelay/interceptor.toml" || fail "update replaced config"
grep -q "v2" "$home_dir/.local/bin/harnessctl" || fail "update did not replace the managed binary"

run_uninstall "$home_dir" >/dev/null
[ ! -e "$home_dir/.local/bin/harnessctl" ] || fail "uninstall left harnessctl"
[ ! -e "$home_dir/.local/bin/harnessd" ] || fail "uninstall left harnessd"
[ -f "$home_dir/.config/harnessrelay/token" ] || fail "uninstall removed config by default"

unmanaged_home="$test_root/unmanaged-home"
mkdir -p "$unmanaged_home/.local/bin"
printf '%s\n' '#!/bin/sh' 'echo unmanaged' >"$unmanaged_home/.local/bin/harnessctl"
chmod 755 "$unmanaged_home/.local/bin/harnessctl"
if run_install "$unmanaged_home" "$source_dir" >/dev/null 2>&1; then
	fail "install overwrote an unmanaged binary"
fi
grep -q unmanaged "$unmanaged_home/.local/bin/harnessctl" || fail "unmanaged binary changed"

symlink_home="$test_root/symlink-home"
mkdir -p "$symlink_home/.config/harnessrelay"
printf '%s\n' "external secret" >"$test_root/external-token"
ln -s "$test_root/external-token" "$symlink_home/.config/harnessrelay/token"
if run_install "$symlink_home" "$source_dir" >/dev/null 2>&1; then
	fail "install accepted a symbolic-link token file"
fi
grep -q "external secret" "$test_root/external-token" || fail "symbolic-link token target changed"

modified_home="$test_root/modified-home"
run_install "$modified_home" "$source_dir" >/dev/null
printf '%s\n' '# locally modified' >>"$modified_home/.local/bin/harnessctl"
if run_uninstall "$modified_home" >/dev/null 2>&1; then
	fail "uninstall removed a modified binary"
fi
[ -f "$modified_home/.local/bin/harnessctl" ] || fail "modified binary was deleted"

purge_home="$test_root/purge-home"
run_install "$purge_home" "$source_dir" >/dev/null
run_uninstall "$purge_home" --purge >/dev/null
[ ! -e "$purge_home/.config/harnessrelay" ] || fail "purge preserved config"
[ ! -e "$purge_home/.local/share/harnessrelay" ] || fail "purge preserved data"

printf '%s\n' "install lifecycle tests passed"
