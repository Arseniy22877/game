#!/bin/bash
# Run against disposable directories on a Docker host (requires sqlite3 and jq).
set -euo pipefail
image=${1:?supply image tag}
tmp=$(mktemp -d)
container=
cleanup() {
    if [[ -n $container ]]; then docker rm -f "$container" >/dev/null; fi
    rm -rf "$tmp"
}
trap cleanup EXIT
mkdir "$tmp/live" "$tmp/restored"
chmod 777 "$tmp/live" "$tmp/restored"
admin=fixture-admin-token
start() {
    container=$(docker run -d -p 127.0.0.1::8080 -v "$1:/data" -e "ADMIN_TOKEN=$admin" "$image")
    base=http://$(docker port "$container" 8080/tcp)
    curl -fsS --retry 15 --retry-all-errors --retry-delay 1 "$base/healthz" | grep -qx ok
}
api() {
    curl -fsS -H "Authorization: Bearer $1" -H 'Content-Type: application/json' "$base/api/$2" "${@:3}"
}
start "$tmp/live"
curl -fsS "$base/admin" | grep -q 'type="module"'
asset=$(find scenarios -type f -name '*.jpg' | sort | sed -n '1p')
path=${asset#scenarios/}
curl -fsS "$base/images/${path%%/*}/${asset##*/}" > "$tmp/image"
cmp "$asset" "$tmp/image"
api "$admin" admin/scenarios | jq -e '.scenarios | map(.id) | contains(["republic_n_scenario","republic_n_scenario_v2"])' >/dev/null
api "$admin" admin/game -X POST >/dev/null
team=$(api '' teams -d '{"name":"Release check","members":["QA"]}' | jq -er .token)
api "$team" team/ready -X PUT -d '{"ready":true}' >/dev/null
api "$admin" admin/game/start -d '{"scenarioId":"republic_n_scenario_v2"}' >/dev/null
api "$team" team/state | jq -Se 'select(.game.round == 1 and .game.phase == "round_open")' > "$tmp/state"
sqlite3 -readonly "$tmp/live/game.db" ".backup '$tmp/restored/game.db'"
chmod 666 "$tmp/restored/game.db"
docker rm -f "$container" >/dev/null
container=
start "$tmp/restored"
api "$team" team/state | jq -S . > "$tmp/replayed"
cmp "$tmp/state" "$tmp/replayed"
