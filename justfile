set dotenv-load

# List recipes
default:
    @just --list

dj := ".domjudge-src"
compose := "docker compose -f docker-compose.yml -f ../compose/dj-override.yml"

dj-clone:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -d "{{dj}}/.git" ]; then
        git -C "{{dj}}" fetch --filter=blob:none origin main
        git -C "{{dj}}" checkout main
        git -C "{{dj}}" reset --hard origin/main
    else
        git clone --filter=blob:none --branch main \
            https://github.com/DOMjudge/domjudge.git "{{dj}}"
    fi

# Start it. First run builds DOMjudge and debootstraps a chroot — several minutes.
dj-up:
    cd {{dj}} && {{compose}} up -d

dj-down:
    cd {{dj}} && {{compose}} down

dj-logs:
    cd {{dj}} && {{compose}} logs -f

dj-shell:
    cd {{dj}} && {{compose}} exec domjudge bash


dj-pass:
    @cat {{dj}}/etc/initial_admin_password.secret

dj-reset:
    cd {{dj}} && {{compose}} down -v
    git -C {{dj}} clean -xfdq
    git -C {{dj}} checkout -- .
