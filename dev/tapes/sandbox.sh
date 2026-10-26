#!/usr/bin/env bash
# Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>
#
# sandbox.sh DIR: builds the demo home the VHS tapes record in (env.sh
# calls it). DIR is emptied first. Everything in it is made up: a small
# Laravel-style project with a git history and an SQLite database, a
# Downloads folder, an archive and a tar that tries to escape its folder.
# The files under ~/projects and ~/.cache take real blocks (about 130 MiB),
# so du-based tools (dirs_largest, dirs_find_large) see them; the big
# downloads are sparse, so the sandbox stays small.
# The PHP files below are written in single quotes, $ and all.
# shellcheck disable=SC2016
set -euo pipefail

root=${1:?usage: sandbox.sh DIR}
case $root in
    /tmp/* | "${RUNNER_TEMP:-/nonexistent}"/*) ;;
    *)
        echo "sandbox.sh: DIR must be under /tmp or \$RUNNER_TEMP" >&2
        exit 2
        ;;
esac
rm -rf "$root"
mkdir -p "$root"
cd "$root"

# A file of SIZE (truncate units): sparse, or with its blocks allocated.
big() { mkdir -p "$(dirname "$1")" && truncate -s "$2" "$1"; }
fat() { mkdir -p "$(dirname "$1")" && fallocate -l "$2" "$1"; }
# A text file with the given lines.
put() { mkdir -p "$(dirname "$1")" && printf '%s\n' "${@:2}" >"$1"; }

# --- ~/projects/shop: the project
p=projects/shop
put $p/composer.json '{' '    "name": "acme/shop",' '    "type": "project",' '    "require": {' '        "php": "^8.3",' '        "laravel/framework": "^11.0"' '    }' '}'
put $p/package.json '{' '  "name": "shop",' '  "private": true,' '  "type": "module",' '  "scripts": { "dev": "vite", "build": "vite build" },' '  "devDependencies": { "vite": "^5.4.0", "laravel-vite-plugin": "^1.0.0" }' '}'
put $p/.gitignore '/vendor' '/node_modules' '.env' '/storage/logs/*.log' '/public/build'
put $p/.env 'APP_NAME=Shop' 'APP_KEY=base64:not-a-real-key-but-a-secret-all-the-same=' 'DB_CONNECTION=sqlite' 'DB_PASSWORD=hunter2' 'STRIPE_SECRET=sk_test_not_a_real_key'
put $p/.env.example 'APP_NAME=Shop' 'APP_KEY=' 'DB_CONNECTION=sqlite' 'DB_PASSWORD=' 'STRIPE_SECRET='
# The front controller answers on its own, so the HTE tape has a page to show.
put $p/public/index.php '<?php' '' 'printf("Acme shop: PHP %s, %s, served by %s\n", PHP_VERSION, $_SERVER["HTTP_HOST"], php_sapi_name());'
put $p/bootstrap/app.php '<?php' '' 'return Illuminate\Foundation\Application::configure(basePath: dirname(__DIR__))->create();'
put $p/routes/web.php '<?php' '' 'use App\Http\Controllers\OrderController;' '' "Route::get('/orders', [OrderController::class, 'index']);" "Route::post('/orders', [OrderController::class, 'store']);"
put $p/app/Http/Controllers/OrderController.php '<?php' '' 'namespace App\Http\Controllers;' '' 'class OrderController extends Controller' '{' '    public function index() { return view("orders.index"); }' '    public function store() { /* TODO: validate */ }' '}'
put $p/app/Http/Controllers/Controller.php '<?php' '' 'namespace App\Http\Controllers;' '' 'abstract class Controller {}'
put $p/app/Models/Order.php '<?php' '' 'namespace App\Models;' '' 'class Order extends \Illuminate\Database\Eloquent\Model {}'
put $p/app/Models/User.php '<?php' '' 'namespace App\Models;' '' 'class User extends \Illuminate\Foundation\Auth\User {}'
put $p/resources/js/app.js "import './bootstrap';" '' "document.querySelectorAll('[data-confirm]').forEach((el) => {" "    el.addEventListener('click', (e) => confirm(el.dataset.confirm) || e.preventDefault());" '});'
put $p/resources/js/bootstrap.js "import axios from 'axios';" 'window.axios = axios;'
put $p/resources/views/orders/index.blade.php '<h1>Orders</h1>'
put $p/README.md '# Shop' '' 'The Acme web shop.'
put $p/artisan '#!/usr/bin/env php' '<?php' '' '// Laravel console entry point.'
put $p/tests/Feature/OrderTest.php '<?php' '' 'test("orders page loads", fn () => $this->get("/orders")->assertOk());'
fat $p/storage/logs/laravel.log 38M
fat $p/public/build/assets/app-4f2a1c.js 1300K
fat $p/vendor/laravel/framework/src/Illuminate/Foundation/Application.php 48K
fat $p/vendor/composer/autoload_classmap.php 2400K
fat $p/node_modules/@esbuild/linux-x64/bin/esbuild 9800K
fat $p/node_modules/rollup/dist/rollup.js 1900K
fat $p/node_modules/vite/dist/node/chunks/dep-CzJTQ5q7.js 2100K
mkdir -p $p/storage/framework/cache/data $p/storage/framework/sessions $p/storage/app/public $p/database/migrations
put $p/database/migrations/2026_01_10_000000_create_orders_table.php '<?php' '' 'return new class extends Migration {};'

sqlite3 $p/database/app.sqlite <<'SQL'
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT NOT NULL UNIQUE, created_at TEXT);
CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER REFERENCES users(id), total_cents INTEGER, status TEXT, created_at TEXT);
INSERT INTO users VALUES (1,'Ada Lovelace','ada@example.com','2026-01-12'),(2,'Alan Turing','alan@example.com','2026-02-03'),(3,'Grace Hopper','grace@example.com','2026-03-21');
INSERT INTO orders VALUES (1,1,4999,'paid','2026-09-01'),(2,1,1250,'paid','2026-09-14'),(3,2,8900,'refunded','2026-09-20'),(4,3,2300,'pending','2026-10-02');
SQL

# The git history: fixed identity and dates, so every recording is the same.
export GIT_CONFIG_GLOBAL="$root/.gitconfig" GIT_CONFIG_NOSYSTEM=1
git config --global user.name "Ada Lovelace"
git config --global user.email ada@example.com
git config --global init.defaultBranch main
git config --global advice.detachedHead false
commit() {
    GIT_AUTHOR_DATE="$1" GIT_COMMITTER_DATE="$1" git -C $p commit -q -m "$2"
}
git -C $p init -q
git -C $p add artisan composer.json package.json .gitignore .env.example README.md public bootstrap
commit "2026-08-03T09:12:00+02:00" "Initial Laravel skeleton"
git -C $p add routes app/Http/Controllers/Controller.php app/Models/User.php tests
commit "2026-08-11T15:40:00+02:00" "Add users"
git -C $p tag -a v1.0.0 -m "Shop 1.0.0"
git -C $p add app/Models/Order.php app/Http/Controllers/OrderController.php database/migrations
commit "2026-09-02T11:05:00+02:00" "Orders: model, controller and migration"
git -C $p add resources
commit "2026-09-18T17:22:00+02:00" "Confirm before destructive buttons"
git -C $p tag -a v1.1.0 -m "Shop 1.1.0"
# origin: a bare repository in the sandbox, one commit behind.
git init -q --bare remotes/shop.git
git -C $p remote add origin "$root/remotes/shop.git"
git -C $p push -q origin main v1.0.0 v1.1.0
git -C $p branch -q -u origin/main
git -C $p add database/app.sqlite
commit "2026-10-06T10:31:00+02:00" "Seed the demo database"
# A change in progress: the prompt shows the tree as dirty.
printf '%s\n' '' '## Development' '' 'npm run dev' >>$p/README.md

# --- ~/.bash_history: what the history tools find
put .bash_history '#1791187200' 'composer install' '#1791187260' 'php artisan migrate' '#1791187320' 'npm run dev' '#1791792000' 'git push origin main' '#1791792120' 'php artisan migrate:fresh --seed' '#1792310400' 'docker compose up -d' '#1792310500' 'php artisan queue:work'

# --- ~/Downloads and ~/archives
big Downloads/ubuntu-24.04.1-desktop-amd64.iso 6100M
big Downloads/backup-2026-09.tar.zst 1400M
big Downloads/Screen\ Recording\ 2026-10-21.mov 820M
big Downloads/invoice-0042.pdf 220K
touch -d 2026-09-12T18:40 Downloads/ubuntu-24.04.1-desktop-amd64.iso
touch -d 2026-10-01T03:00 Downloads/backup-2026-09.tar.zst
touch -d 2026-10-21T16:02 Downloads/Screen\ Recording\ 2026-10-21.mov
touch -d 2026-10-15T09:27 Downloads/invoice-0042.pdf
fat .cache/composer/files/laravel-framework.zip 9200K
fat .cache/pip/http/b/4/e/wheel.bin 64M
mkdir -p archives/site
put archives/site/index.html '<h1>It works</h1>'
put archives/site/css/site.css 'body { font-family: system-ui; }'
tar -czf archives/site-1.4.2.tar.gz -C archives site
rm -rf archives/site
# A tar whose member climbs out of the extraction folder.
mkdir -p .evil/a
put .evil/payload.sh 'echo pwned'
tar -cPf archives/suspicious.tar -C .evil/a ../payload.sh
rm -rf .evil
