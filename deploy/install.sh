#!/bin/bash
# Run as root on a dedicated Ubuntu VPS with Docker Compose, Caddy and sqlite3.
set -euo pipefail
cd "$(dirname "$0")"
domain=${1:?public hostname}
pubkey=${2:?path to the public CI key}
[[ $domain =~ ^[a-zA-Z0-9.-]+$ ]]
ssh-keygen -lf "$pubkey" >/dev/null
docker compose version
base=/opt/cu-game
install -d -m 755 "$base"
install -d -m 700 "$base/data"
chown 10001:10001 "$base/data"
install -d -m 700 "$base/backup"
install -m 644 compose.yaml "$base/"
install -m 755 release.sh backup.sh "$base/"
if [[ ! -e $base/.env ]]; then
    (umask 077; printf 'ADMIN_TOKEN=%s\n' "$(openssl rand -hex 32)" > "$base/.env")
fi
getent passwd cu-deploy >/dev/null || useradd --system --create-home --shell /bin/sh cu-deploy
home_dir=$(getent passwd cu-deploy | cut -d: -f6)
install -d -m 700 -o cu-deploy -g cu-deploy "$home_dir/.ssh"
printf 'restrict,command="sudo -n /opt/cu-game/release.sh" %s\n' "$(cat "$pubkey")" > "$home_dir/.ssh/authorized_keys"
chown cu-deploy:cu-deploy "$home_dir/.ssh/authorized_keys"
chmod 600 "$home_dir/.ssh/authorized_keys"
printf '%s\n' 'Defaults:cu-deploy env_keep += "SSH_ORIGINAL_COMMAND"' 'cu-deploy ALL=(root) NOPASSWD: /opt/cu-game/release.sh ""' > /etc/sudoers.d/cu-game
chmod 440 /etc/sudoers.d/cu-game
visudo -cf /etc/sudoers.d/cu-game
install -m 644 cu-game.service cu-game-backup.{service,timer} /etc/systemd/system/
sed "s/{\$GAME_DOMAIN}/$domain/" Caddyfile > /etc/caddy/Caddyfile
caddy validate --config /etc/caddy/Caddyfile
systemctl daemon-reload
systemctl enable --now docker caddy cu-game-backup.timer
systemctl reload caddy
systemctl enable cu-game.service
