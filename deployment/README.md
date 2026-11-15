# Local Nginx and middleware deployment reference

This reference matches the current development setup: Go/Gin runs on the host at
`127.0.0.1:8080`; MySQL, Redis, and RabbitMQ run in Docker; Nginx serves the HTML
files and proxies `/api/` requests to Gin. It does not containerize the Go API.

## 1. Register page

The frontend is a Vue 3 + Vite single-page app. The home view calls the protected
`GET /api/v1/feed?limit=5`, adds another five items when the scroll sentinel
appears, and uses `next_cursor` for the next request. The question view calls the
content detail, answer list, and comment APIs. Login/register save the access
token; the shared API helper then sends it as a Bearer token.

For local development, start the backend and run `npm install` then
`npm run dev` inside `frontend/`. Use Node.js 20.19+ or 22.12+ for current Vite.
Vite proxies `/api/` to `127.0.0.1:8080`, so the browser and API remain
same-origin during development:

```bash
cd frontend
npm install
npm run dev
```

Open the Vite URL (normally `http://127.0.0.1:5173`). For a production build,
run `npm run build`; Nginx serves the generated `frontend/dist` directory.
The old `gpt_test.html`, `sec.html`, and standalone `register.html` are retained
as prototype references; the Vue app routes are `/`, `/question/:id`, `/login`,
and `/register`.

## 2. Run MySQL, Redis, and RabbitMQ with Compose

Docker Engine and the Compose plugin must be installed. Check them with:

```bash
docker --version
docker compose version
```

If native MySQL, Redis, or RabbitMQ services are already running on the same
machine, stop them first to free ports 3306, 6379, and 5672. For example:

```bash
sudo systemctl stop redis-server
sudo systemctl stop rabbitmq-server
sudo systemctl stop mysql
```

The MySQL container gets a new, separate named volume; it does not import data
from an existing native MySQL installation. If you need that existing data,
export/import it before switching, or map the container to another host port
(for example `127.0.0.1:3307:3306`) and set `db.port: 3307` instead.

From the repository root:

```bash
cp deployment/.env.example deployment/.env
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml config
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml up -d
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml ps
```

All database/middleware ports are published only on host loopback. MySQL stores
its files in the named `mysql_data` volume and creates the `myzhihu` database
and `myzhihu_app` account when that volume is initialized for the first time.
The RabbitMQ management page is available at `http://127.0.0.1:15672`; sign in
with the credentials in `deployment/.env`.

Update `backend/settings/settings.yaml` to use the application DB account. The
Go config struct maps its `Password` field from the YAML key `passport`, so keep
that spelling:

```yaml
db:
  user: myzhihu_app
  passport: myzhihu_local_change_me
  host: "127.0.0.1"
  port: 3306
  db_name: myzhihu
redis:
  enabled: true
  host: "127.0.0.1"
  port: 6379
  password: ""
  db: 0
rabbitmq:
  enabled: true
  host: "127.0.0.1"
  port: 5672
  username: "myzhihu2" # must match deployment/.env
  password: "myzhihu2" # must match deployment/.env
  vhost: "/"
```

`MYSQL_ROOT_PASSWORD` is only for MySQL administration and the health check.
The application connects as `MYSQL_USER` with `MYSQL_PASSWORD`; keep those
values aligned with `db.user` and `db.passport` above.

The Go startup code declares the exchange, two durable queues, and bindings:

```text
myzhihu2.events (direct)
  ├─ user.behavior → myzhihu2.user_behavior
  └─ feed.content  → myzhihu2.feed
```

No extra queue setup is required by hand. Verify the services with:

```bash
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml exec redis redis-cli ping
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml exec mysql sh -c 'mysqladmin ping -h 127.0.0.1 -uroot -p"$MYSQL_ROOT_PASSWORD" --silent'
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml exec rabbitmq rabbitmq-diagnostics -q ping
docker compose --env-file deployment/.env -f deployment/compose.middleware.yml logs -f redis rabbitmq
```

`docker compose ... down` stops the containers but retains named data volumes.
`docker compose ... down -v` also deletes MySQL, Redis, and RabbitMQ data, so use
it only when intentionally resetting the local environment. MySQL's
`MYSQL_DATABASE`, `MYSQL_USER`, and passwords are initialization settings: after
the data volume exists, changing `.env` does not recreate users or reset their
passwords.

Once all three services report healthy, create the project's five tables using
the backend migration flag (run this from the `backend` directory):

```bash
go run ./cmd/main.go -f settings/settings.yaml -db
```

Then start the backend normally with `go run ./cmd/main.go -f settings/settings.yaml`.

## 3. Install and configure Nginx on Ubuntu

Install Nginx:

```bash
sudo apt update
sudo apt install -y nginx
```

Build the Vue app, then copy Vite's `dist` output to a stable web root. Nginx's
worker must be able to read the files:

```bash
cd frontend
npm install
npm run build
cd ..
sudo install -d -o root -g www-data -m 755 /var/www/myzhihu/frontend/dist
sudo cp -a frontend/dist/. /var/www/myzhihu/frontend/dist/
```

Install the example server block and enable it:

```bash
sudo install -m 644 deployment/nginx/myzhihu.conf.example /etc/nginx/sites-available/myzhihu
sudo ln -s /etc/nginx/sites-available/myzhihu /etc/nginx/sites-enabled/myzhihu
```

If the packaged default site is also enabled as a `default_server`, disable its
symlink so it does not conflict with this server block (`sudo unlink
/etc/nginx/sites-enabled/default`, if that link exists). Then validate and reload:

```bash
sudo nginx -t
sudo systemctl enable --now nginx
sudo systemctl reload nginx
```

Start the Go backend from `backend/` in another terminal:

```bash
cd backend
go run ./cmd/main.go -f settings/settings.yaml
```

The current `settings.yaml` binds Gin to `127.0.0.1:8080`, which is reachable by
Nginx on the same host. Open `http://127.0.0.1/` for the home page,
`http://127.0.0.1/register` for registration, or `/question/<id>` for a question.
Nginx's `/api/` location forwards API requests unchanged to Gin. Its SPA fallback
serves `index.html` for Vue Router paths while preserving the browser URL.

Check the route and logs:

```bash
curl -I http://127.0.0.1/
curl -I http://127.0.0.1/register
curl -I http://127.0.0.1/question/1
sudo tail -f /var/log/nginx/access.log /var/log/nginx/error.log
```

For a remote server, configure a real `server_name`, DNS, HTTPS, and firewall
rules before exposing port 80/443. Keep Redis and RabbitMQ ports private; this
Compose example binds them only to `127.0.0.1`.

## 4. Later: move the Go API into Compose

This project is not yet configured to build/run the Go API as a container. If
you later add the backend service to Compose, containers must connect using
service DNS names (for example `redis:6379` and `rabbitmq:5672`), not
`127.0.0.1`. The backend must listen on `0.0.0.0` inside its container, and
Nginx's `proxy_pass` should target the backend service name. The current example
intentionally keeps the backend on the host to match the present development
setup.

## 5. Run the project with locally installed MySQL, Redis, and RabbitMQ

Use this workflow when the three middleware services are installed directly on
the host rather than started from the Compose file above. This matches the
current `backend/settings/settings.yaml`, which connects to `127.0.0.1` on the
standard ports. Do not start the Compose middleware at the same time if it would
bind those same host ports.

### 5.1 Check the local services

Check service-manager status:

```bash
sudo systemctl status mysql --no-pager
sudo systemctl status redis-server --no-pager
sudo systemctl status rabbitmq-server --no-pager
```

If a service is installed but stopped, start it:

```bash
sudo systemctl start mysql
sudo systemctl start redis-server
sudo systemctl start rabbitmq-server
```

Probe each service using the same host and ports configured by the backend:

```bash
redis-cli -h 127.0.0.1 -p 6379 ping
sudo rabbitmq-diagnostics -q ping
mysqladmin --protocol=tcp -h 127.0.0.1 -P 3306 -u root -p ping
```

Expected responses include `PONG`, RabbitMQ's `Ping succeeded`, and
`mysqld is alive`. `mysqladmin -p` prompts for the MySQL root password; the
current local settings file uses `root` as both the configured database user and
password, so enter that only if it is still the actual local account password.

The app settings must match the services and credentials on this host:

```yaml
db:
  user: root
  passport: root
  host: "127.0.0.1"
  port: 3306
  db_name: myzhihu
redis:
  enabled: true
  host: "127.0.0.1"
  port: 6379
  password: ""
  db: 0
rabbitmq:
  enabled: true
  host: "127.0.0.1"
  port: 5672
  username: "myzhihu2"
  password: "myzhihu2"
  vhost: "/"
```

For a shared development machine, create a dedicated MySQL application account
and use its credentials in `db.user` and `db.passport`; avoid running the app as
the MySQL root account. Likewise, the RabbitMQ user must exist and have
configure/write/read permissions on the configured `/` vhost. The backend
declares `myzhihu2.events`, `myzhihu2.user_behavior`, and `myzhihu2.feed` during
startup.

### 5.2 Migrate and run the Go backend

Run from the backend directory so the default settings path resolves correctly:

```bash
cd backend
go run ./cmd/main.go -f settings/settings.yaml -db
```

The migration command connects to MySQL and creates/updates the five model
tables. Then start the HTTP server in a terminal:

```bash
go run ./cmd/main.go -f settings/settings.yaml
```

The backend listens on `127.0.0.1:8080`. Since Redis and RabbitMQ are enabled in
the settings, both must be reachable for startup; the Feed consumers begin
consuming the configured RabbitMQ queues after the connections initialize.

### 5.3 Run the Vue frontend in development mode

In another terminal, from the repository root:

```bash
cd frontend
npm install
npm run dev
```

Open the Vite URL printed in the terminal, normally `http://127.0.0.1:5173`.
`frontend/vite.config.js` proxies `/api/` to `http://127.0.0.1:8080`, so the Vue
pages can use the real backend while the middleware stays local. Register or
log in first; the homepage Feed requires the JWT returned by authentication.

### 5.4 Serve the production frontend through local Nginx

When switching from Vite dev server to the Nginx reverse proxy, build the SPA
and copy its output to the configured web root:

```bash
cd frontend
npm run build
cd ..
sudo install -d -o root -g www-data -m 755 /var/www/myzhihu/frontend/dist
sudo cp -a frontend/dist/. /var/www/myzhihu/frontend/dist/
sudo nginx -t
sudo systemctl reload nginx
```

Keep the Go server running on `127.0.0.1:8080`. Nginx serves the Vue app at
`http://127.0.0.1/`, forwards `/api/` to Gin, and returns the SPA entry point
for routes such as `/question/123` and `/register`. Useful checks are:

```bash
curl -I http://127.0.0.1/
curl -i http://127.0.0.1/api/v1/feed?limit=5
sudo tail -f /var/log/nginx/access.log /var/log/nginx/error.log
```

The Feed request without a Bearer token should return HTTP 401; after logging in,
the browser sends `Authorization: Bearer <access_token>` automatically through
the Vue API helper.
