#!/bin/bash
set -euo pipefail
cd /opt/cu-game
read -r action run attempt sha extra <<< "${SSH_ORIGINAL_COMMAND:-}"
[[ $action == release && $run =~ ^[1-9][0-9]{0,12}$ && $attempt =~ ^[1-9][0-9]{0,5}$ && $sha =~ ^[0-9a-f]{40}$ && -z $extra ]]
exec 9>.deploy.lock
flock 9
previous=
if [[ -f release.env ]]; then
    source release.env
    if (( run < RUN_NUMBER || (run == RUN_NUMBER && attempt < RUN_ATTEMPT) )); then
        echo 'Superseded deployment' >&2
        exit 1
    fi
    previous=$RELEASE
fi
docker load
docker image inspect "cu-game:$sha" >/dev/null
if [[ -f data/game.db ]]; then ./backup.sh; fi
next=$(mktemp .release.XXXXXX)
trap 'rm -f "$next"' EXIT
printf 'RELEASE=%s\nRUN_NUMBER=%s\nRUN_ATTEMPT=%s\n' "$sha" "$run" "$attempt" > "$next"
activate() {
    docker compose --env-file "$1" up -d &&
        curl --retry 20 --retry-all-errors --retry-delay 1 --max-time 2 -fsS http://127.0.0.1:8080/healthz | grep -qx ok
}
if ! activate "$next"; then
    if [[ -n $previous ]]; then activate release.env; else docker compose --env-file "$next" stop; fi
    exit 1
fi
mv "$next" release.env
systemctl start cu-game.service
while read -r tag; do
    if [[ $tag =~ ^[0-9a-f]{40}$ && $tag != "$sha" && $tag != "$previous" ]]; then
        docker image rm "cu-game:$tag"
    fi
done < <(docker image ls cu-game --format '{{.Tag}}')
