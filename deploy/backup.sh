#!/bin/bash
set -euo pipefail
cd /opt/cu-game
umask 077
exec 8>backup/.lock
flock 8
tmp=$(mktemp backup/.pending.XXXXXX)
trap 'rm -f "$tmp"' EXIT
sqlite3 -cmd '.timeout 10000' data/game.db ".backup '$tmp'"
[[ $(sqlite3 "$tmp" 'PRAGMA integrity_check') == ok ]]
mv "$tmp" "backup/game-$(date -u +%FT%H%M%S%N).db"
find backup -name 'game-*.db' -type f -mmin +20160 -delete
