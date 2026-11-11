#!/usr/bin/env bash
# Check a running server from the outside: rate limit, no shell, no scp or
# sftp, no forwarding, and the per-address session cap.
#
#   scripts/check-live.sh [host]            # default ssh.devops-daily.com
#   SSH_CONFIG=./test_config scripts/check-live.sh ssh.devops-daily.com
#
# SSH_CONFIG points ssh, scp and sftp at a config file, for example one with a
# HostName line before DNS exists. Takes about four minutes: it waits for the
# one-minute rate window to clear between sections. Run it from one address
# and do not connect from that address meanwhile.
set -u
HOST=${1:-ssh.devops-daily.com}
CFG=()
[ -n "${SSH_CONFIG:-}" ] && CFG=(-F "$SSH_CONFIG")
ADDR=$(ssh "${CFG[@]}" -G "$HOST" | awk '/^hostname /{print $2}')
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
fail=0

banner() { timeout 5 bash -c 'exec 3<>"/dev/tcp/$0/22" && IFS= read -r -t 4 l <&3 && printf "%s" "$l"' "$ADDR" 2>/dev/null | tr -d '\r'; }
section() { sleep 65; echo; echo "=== $* ($(date -u +%H:%M:%S) UTC)"; }
check() { if eval "$2"; then echo "PASS  $1"; else echo "FAIL  $1"; fail=1; fi; }

section "rate limit: 11 new connections inside a minute"
for i in $(seq 1 11); do echo "$(banner)" > "$TMP/b$i"; printf '  %2d: %s\n' "$i" "$(cat "$TMP/b$i")"; done
check "connections 1-10 get the SSH banner" '[ "$(grep -l "^SSH-2.0-" "$TMP"/b{1..10} | wc -l)" = 10 ]'
check "connection 11 is refused" 'grep -q "Too many connections" "$TMP/b11"'

section "commands, scp, sftp, forwarding"
ssh "${CFG[@]}" "$HOST" cat /etc/passwd > "$TMP/exec" 2>&1; rc=$?
check "ssh host cat /etc/passwd runs nothing (exit $rc)" '[ $rc = 1 ] && ! grep -q "^root:" "$TMP/exec"'
scp "${CFG[@]}" -O "$HOST:/etc/passwd" "$TMP/passwd" > /dev/null 2>&1; rc=$?
check "scp -O download fails (exit $rc)" '[ $rc != 0 ] && [ ! -s "$TMP/passwd" ]'
scp "${CFG[@]}" /etc/hostname "$HOST:/tmp/x" > "$TMP/scp" 2>&1; rc=$?
check "scp (sftp mode) fails (exit $rc)" '[ $rc != 0 ] && grep -q "subsystem request failed" "$TMP/scp"'
sftp "${CFG[@]}" -b /dev/null "$HOST" > "$TMP/sftp" 2>&1; rc=$?
check "sftp fails (exit $rc)" '[ $rc != 0 ] && grep -q "subsystem request failed" "$TMP/sftp"'
timeout 20 ssh "${CFG[@]}" -N -o ExitOnForwardFailure=yes -R 18081:127.0.0.1:22 "$HOST" > "$TMP/R" 2>&1
check "ssh -R is refused" 'grep -q "remote port forwarding failed" "$TMP/R"'
ssh "${CFG[@]}" -N -L 18080:127.0.0.1:22 "$HOST" 2> "$TMP/L" & pid=$!
sleep 3
timeout 5 bash -c 'exec 3<>/dev/tcp/127.0.0.1/18080; head -c 1 <&3' > "$TMP/Lout" 2>/dev/null
kill $pid 2>/dev/null; wait $pid 2>/dev/null
check "ssh -L gets nothing through the tunnel" '[ ! -s "$TMP/Lout" ] && grep -q "open failed" "$TMP/L"'

section "5 open sessions per address"
for i in 1 2 3 4 5; do (sleep 30 | timeout 12 ssh "${CFG[@]}" -tt "$HOST" > /dev/null 2>&1) & done
sleep 5
b=$(banner)
echo "  6th: $b"
check "a 6th connection is refused while 5 are open" '[[ "$b" == *"Too many connections"* ]]'
wait 2>/dev/null

echo
[ $fail = 0 ] && echo "all checks passed" || echo "SOME CHECKS FAILED"
exit $fail
