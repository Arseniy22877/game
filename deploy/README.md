# Выкладка игры

В PR workflow Game запускает Go-проверки, собирает образ amd64, открывает тестовый раунд и сравнивает состояние команды с результатом восстановления SQLite-копии. После merge в main раннер передаёт тот же проверенный образ на VPS по SSH. Caddy обслуживает HTTPS и WSS, Go слушает на loopback хоста через Docker. Данные, секреты и последний успешный SHA лежат в /opt/cu-game. Сертификаты сохраняются в хранилище системной службы Caddy.

## Установка

На выделенном Ubuntu amd64 нужны Docker с Compose, systemd, caddy, sqlite3, openssl, sudo и curl. Проверьте DNS, свободные TCP 80/443 и доступность SSH с GitHub-hosted runners. Install-скрипт управляет системным Caddyfile и ключом отдельного пользователя cu-deploy. На занятом сервере сначала согласуйте конфигурацию существующих сайтов.

1. Создайте отдельную пару ключей через `ssh-keygen -t ed25519 -f ci-game`. Для CI задайте пустую парольную фразу. Скопируйте на VPS каталог deploy и открытый ключ.
2. От root выполните `bash deploy/install.sh <домен> /путь/ci-game.pub`. Для адреса sslip.io укажите IPv4 с дефисами, затем .sslip.io.
3. Задайте репозиторные Actions secrets через stdin командой `gh secret set ИМЯ`. DEPLOY_HOST содержит адрес SSH, DEPLOY_KEY — закрытый ключ CI, DEPLOY_KNOWN_HOSTS — проверенную строку known_hosts для этого адреса. Первую выкладку запускает merge в main.
4. Откройте /admin. Владелец может посмотреть токен в приватном SSH-терминале командой `sudo cat /opt/cu-game/.env`. Командам передайте корневой HTTPS-адрес.

## Эксплуатация

Из /opt/cu-game используйте `sudo docker compose --env-file release.env logs --tail 100 game`. Для Caddy и бэкапов используйте `journalctl -u caddy -u cu-game-backup`. Дежурная проверка — GET /healthz. Команда `systemctl restart cu-game` пересоздаёт процесс с тем же каталогом данных. Изменения хостовых файлов deploy устанавливаются повторным запуском install.sh с тем же открытым ключом.

При неуспешном обновлении скрипт запускает предыдущий образ. Для ручного отката найдите предыдущий SHA через `docker image ls cu-game`, измените только RELEASE в release.env и выполните `systemctl restart cu-game`. Не удаляйте data или .env.

Суточный systemd timer и скрипт релиза вызывают backup.sh. SQLite делает онлайн-копию, проверяет целостность и публикует файл в /opt/cu-game/backup. Копии старше 14 дней удаляются после успешной записи. Для восстановления остановите cu-game, переименуйте каталог data в отдельный резервный каталог, создайте пустой data с владельцем 10001:10001 и правами 0700, установите выбранную копию как data/game.db с тем же владельцем и правами 0600 и запустите cu-game. Сверьте раунд через /admin.

Для локальной приёмки нужны Docker, Bash, curl, jq и sqlite3. Из корня выполните `docker build -t cu-game:local .` и `bash deploy/smoke.sh cu-game:local`.
