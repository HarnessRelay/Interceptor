#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(dirname -- "$script_dir")
source_bin_dir=${HARNESSRELAY_SOURCE_BIN_DIR:-"$repo_dir/bin"}
bin_dir=${HARNESSRELAY_BIN_DIR:-"${HOME:?HOME is required}/.local/bin"}
config_home=${XDG_CONFIG_HOME:-"$HOME/.config"}
data_home=${XDG_DATA_HOME:-"$HOME/.local/share"}
state_home=${XDG_STATE_HOME:-"$HOME/.local/state"}
config_dir="$config_home/harnessrelay"
data_dir="$data_home/harnessrelay"
state_dir="$state_home/harnessrelay"
manifest="$config_dir/install-manifest"
token_file="$config_dir/token"
config_file="$config_dir/interceptor.toml"
skip_build=0
force=0
mode=installed

usage() {
	printf '%s\n' "Usage: scripts/install.sh [--skip-build] [--force] [--update]"
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--skip-build) skip_build=1 ;;
		--force) force=1 ;;
		--update) mode=updated ;;
		--help|-h) usage; exit 0 ;;
		*) usage >&2; printf 'install: unknown option: %s\n' "$1" >&2; exit 2 ;;
	esac
	shift
done

hash_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		printf '%s\n' "install: sha256sum or shasum is required" >&2
		exit 1
	fi
}

manifest_value() {
	key=$1
	[ -f "$manifest" ] || return 1
	awk -F= -v key="$key" '$1 == key {sub(/^[^=]*=/, ""); print; exit}' "$manifest"
}

if [ "$skip_build" -eq 0 ]; then
	(cd "$repo_dir" && make build)
fi

for name in harnessctl harnessd; do
	if [ ! -f "$source_bin_dir/$name" ] || [ ! -x "$source_bin_dir/$name" ]; then
		printf 'install: missing executable build output: %s\n' "$source_bin_dir/$name" >&2
		exit 1
	fi
done

umask 077
mkdir -p "$bin_dir" "$config_dir" "$data_dir" "$state_dir"
chmod 700 "$config_dir" "$data_dir" "$state_dir"

for name in harnessctl harnessd; do
	destination="$bin_dir/$name"
	if [ -e "$destination" ] && [ "$force" -ne 1 ]; then
		recorded_path=$(manifest_value "${name}_path" || true)
		recorded_hash=$(manifest_value "${name}_sha256" || true)
		current_hash=$(hash_file "$destination")
		if [ "$recorded_path" != "$destination" ] || [ -z "$recorded_hash" ] || [ "$recorded_hash" != "$current_hash" ]; then
			printf 'install: refusing to overwrite unmanaged or modified file: %s\n' "$destination" >&2
			printf '%s\n' "Use --force only after inspecting that exact file." >&2
			exit 1
		fi
	fi
done

for name in harnessctl harnessd; do
	destination="$bin_dir/$name"
	tmp=$(mktemp "$bin_dir/.${name}.XXXXXX")
	trap 'rm -f "$tmp"' EXIT HUP INT TERM
	cp "$source_bin_dir/$name" "$tmp"
	chmod 755 "$tmp"
	mv -f "$tmp" "$destination"
	trap - EXIT HUP INT TERM
done

if [ -L "$token_file" ]; then
	printf 'install: refusing symbolic-link token file: %s\n' "$token_file" >&2
	exit 1
elif [ ! -f "$token_file" ]; then
	token_tmp=$(mktemp "$config_dir/.token.XXXXXX")
	trap 'rm -f "$token_tmp"' EXIT HUP INT TERM
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -base64 32 >"$token_tmp"
	else
		od -An -N32 -tx1 /dev/urandom | tr -d ' \n' >"$token_tmp"
		printf '\n' >>"$token_tmp"
	fi
	chmod 600 "$token_tmp"
	mv "$token_tmp" "$token_file"
	trap - EXIT HUP INT TERM
else
	chmod 600 "$token_file"
fi

if [ ! -e "$config_file" ]; then
	config_tmp=$(mktemp "$config_dir/.interceptor.XXXXXX")
	trap 'rm -f "$config_tmp"' EXIT HUP INT TERM
	{
		printf '%s\n' '# HarnessRelay user-local configuration'
		printf '%s\n' '# Environment variables override these documented defaults.'
		printf '%s\n' 'bind_address = "127.0.0.1"'
		printf '%s\n' 'port = 8765'
	} >"$config_tmp"
	chmod 600 "$config_tmp"
	mv "$config_tmp" "$config_file"
	trap - EXIT HUP INT TERM
fi

# Best-effort managed cloudflared download for the remote-access tunnel.
# Failure is never fatal: the dashboard can download it later (Settings >
# Tunnel). HARNESSRELAY_SKIP_CLOUDFLARED_DOWNLOAD=1 skips this step.
cloudflared_dir="$data_dir/bin"
cloudflared_bin="$cloudflared_dir/cloudflared"
if [ ! -f "$cloudflared_bin" ] && [ "${HARNESSRELAY_SKIP_CLOUDFLARED_DOWNLOAD:-}" != "1" ]; then
	case "$(uname -m)" in
		x86_64) cloudflared_arch=amd64 ;;
		aarch64|arm64) cloudflared_arch=arm64 ;;
		*) cloudflared_arch="" ;;
	esac
	if [ -n "$cloudflared_arch" ] && command -v curl >/dev/null 2>&1; then
		cloudflared_url=${HARNESSRELAY_CLOUDFLARED_DOWNLOAD_URL:-"https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-$cloudflared_arch"}
		mkdir -p "$cloudflared_dir"
		cloudflared_tmp=$(mktemp "$cloudflared_dir/.cloudflared.XXXXXX")
		if curl -fsSL --max-time 300 -o "$cloudflared_tmp" "$cloudflared_url"; then
			chmod 755 "$cloudflared_tmp"
			if "$cloudflared_tmp" --version >/dev/null 2>&1; then
				mv -f "$cloudflared_tmp" "$cloudflared_bin"
			else
				rm -f "$cloudflared_tmp"
				printf '%s\n' "install: downloaded cloudflared failed its version check; skipped (download it later from the dashboard)"
			fi
		else
			rm -f "$cloudflared_tmp"
			printf '%s\n' "install: cloudflared download failed; skipped (download it later from the dashboard)"
		fi
	else
		printf '%s\n' "install: cloudflared not downloaded (unsupported arch or curl missing); download it later from the dashboard"
	fi
fi

manifest_tmp=$(mktemp "$config_dir/.install-manifest.XXXXXX")
trap 'rm -f "$manifest_tmp"' EXIT HUP INT TERM
{
	printf '%s\n' "version=1"
	printf 'harnessctl_path=%s\n' "$bin_dir/harnessctl"
	printf 'harnessctl_sha256=%s\n' "$(hash_file "$bin_dir/harnessctl")"
	printf 'harnessd_path=%s\n' "$bin_dir/harnessd"
	printf 'harnessd_sha256=%s\n' "$(hash_file "$bin_dir/harnessd")"
} >"$manifest_tmp"
chmod 600 "$manifest_tmp"
mv "$manifest_tmp" "$manifest"
trap - EXIT HUP INT TERM

"$bin_dir/harnessctl" --help >/dev/null
"$bin_dir/harnessd" --help >/dev/null

printf '\nHarnessRelay %s.\n\n' "$mode"
printf 'Binaries:\n  %s\n  %s\n\n' "$bin_dir/harnessctl" "$bin_dir/harnessd"
printf 'Config:\n  %s\n  stable token: %s (0600)\n\n' "$config_file" "$token_file"
if [ -f "$cloudflared_bin" ]; then
	printf 'Tunnel binary:\n  %s\n\n' "$cloudflared_bin"
fi
case ":${PATH:-}:" in
	*":$bin_dir:"*) printf '%s\n\n' "CLI PATH: ready" ;;
	*)
		printf '%s\n' "CLI PATH: $bin_dir is not in PATH."
		printf '%s\n\n' "Add this to your shell profile, then restart the shell:"
		printf '  export PATH="%s:$PATH"\n\n' "$bin_dir"
		;;
esac
printf '%s\n' "Next steps:"
printf '%s\n' "  1. Install the user service: harnessctl services install"
printf '%s\n' "  2. Start it now: harnessctl services start"
printf '%s\n' "  3. Start it at login: harnessctl services enable"
printf '%s\n' "  4. Check status: harnessctl status"
printf '%s\n' "  5. Install shims: harnessctl shims install codex opencode"
printf '%s\n' '  6. Activate shims: export PATH="$(harnessctl shims path):$PATH"'
printf '%s\n' "Manual fallback: harnessd serve"
printf '\n%s\n' "No shell profile or harness shim was changed."
