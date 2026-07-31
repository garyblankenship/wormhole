#!/bin/sh

set -eu

is_forbidden_path() {
	case "$1" in
		.claude | .claude/* | */.claude | */.claude/* | \
			.work | .work/* | \
			logs | logs/* | \
			tmp | tmp/* | \
			pkg/wormhole | pkg/wormhole/*)
			return 0
			;;
	esac
	return 1
}

check_inventory() {
	label=$1
	inventory=$2
	found=0

	while IFS= read -r path; do
		if is_forbidden_path "$path"; then
			printf '%s contains forbidden path: %s\n' "$label" "$path" >&2
			found=1
		fi
	done <"$inventory"

	if [ "$found" -ne 0 ]; then
		return 1
	fi
}

check_export_ignore() {
	path=$1
	result=$(git check-attr export-ignore -- "$path")
	case "$result" in
		*": export-ignore: set")
			return 0
			;;
	esac
	printf 'export-ignore is not set recursively for: %s\n' "$path" >&2
	return 1
}

check_ignored() {
	path=$1
	if git check-ignore -q -- "$path"; then
		return 0
	fi
	printf 'local-only path is not ignored: %s\n' "$path" >&2
	return 1
}

surface_tmp=$(mktemp -d "${TMPDIR:-/tmp}/wormhole-public-surface.XXXXXX")
trap 'rm -rf "$surface_tmp"' EXIT HUP INT TERM

git ls-files >"$surface_tmp/tracked"
check_inventory "tracked files" "$surface_tmp/tracked"

git archive --worktree-attributes --format=tar HEAD |
	tar -tf - >"$surface_tmp/archive"
check_inventory "source archive" "$surface_tmp/archive"

check_ignored ".claude/public-surface-sentinel"
check_ignored "nested/.claude/public-surface-sentinel"
check_ignored ".work/public-surface-sentinel"
check_ignored "logs/public-surface-sentinel"
check_ignored "tmp/public-surface-sentinel"
check_ignored "pkg/wormhole/public-surface-sentinel"

check_export_ignore ".claude/public-surface-sentinel"
check_export_ignore "nested/.claude/public-surface-sentinel"
check_export_ignore ".work/public-surface-sentinel"
check_export_ignore "logs/public-surface-sentinel"
check_export_ignore "tmp/public-surface-sentinel"
check_export_ignore "pkg/wormhole/public-surface-sentinel"

check_removed_api() {
	pattern=$1
	if git grep -nE "$pattern" -- '*.go' ':(exclude)*_test.go' >/dev/null; then
		printf 'removed v3 API is still present: %s\n' "$pattern" >&2
		git grep -nE "$pattern" -- '*.go' ':(exclude)*_test.go' >&2
		return 1
	fi
}

check_removed_api '(^|[^[:alnum:]_])(PIDConfig|PIDController|DefaultPIDConfig|NewPIDController)([^[:alnum:]_]|$)'
check_removed_api 'WithMiddleware|LegacyAdapter|type Middleware |type Handler |type Chain '
check_removed_api 'RetryMiddleware|LoadBalancerMiddleware|BuildToolResultMessage\('
check_removed_api 'MaxMemoryMB|MaxCPUTime|EnableResourceIsolation|JournalEntry'
check_removed_api 'func \(.*\*Wormhole\) Provider\('

printf '%s\n' "public surface check passed"
