set dotenv-load

dj := "docker compose -f dev/domjudge/compose.yaml"

# List recipes
default:
    @just --list

# Start the local DOMjudge (web UI and API on http://localhost:12345)
dj-up:
    {{dj}} up -d mariadb domserver
    @echo "waiting for domserver..."
    @until {{dj}} exec -T domserver test -f /opt/domjudge/domserver/etc/restapi.secret 2>/dev/null \
        && curl -sf -o /dev/null http://localhost:12345/api/v4/info; do sleep 3; done
    @printf 'JUDGEDAEMON_PASSWORD=%s\n' \
        "$({{dj}} exec -T domserver awk '!/^#/ && NF {print $NF}' /opt/domjudge/domserver/etc/restapi.secret)" \
        > dev/domjudge/.env
    {{dj}} --profile judge up -d judgehost
    @just dj-admin

# Print the local DOMjudge admin password
dj-admin:
    @printf 'admin password: '
    @{{dj}} exec -T domserver cat /opt/domjudge/domserver/etc/initial_admin_password.secret

# Submit a correct solution to the demo contest (problems: hello, fltcmp, boolfind)
dj-solve problem team="exteam":
    #!/usr/bin/env sh
    set -eu
    dir=/opt/domjudge/domserver/example_problems/{{problem}}/submissions/accepted
    file=$({{dj}} exec -T domserver sh -c "ls $dir/*.c | head -1")
    tmp=$(mktemp -d)
    {{dj}} exec -T domserver cat "$file" > "$tmp/solution.c"
    pass=$({{dj}} exec -T domserver cat /opt/domjudge/domserver/etc/initial_admin_password.secret)
    if ! out=$(curl -s --fail-with-body -u "admin:$pass" -X POST http://localhost:12345/api/v4/contests/demo/submissions \
        -F problem_id={{problem}} -F language_id=c -F team_id={{team}} \
        -F "code[]=@$tmp/solution.c;filename=solution.c"); then
        rm -r "$tmp"
        echo "submission rejected: $out" >&2
        exit 1
    fi
    rm -r "$tmp"
    echo "submitted {{problem}} for {{team}}; judging takes a few seconds"

# Stop the local DOMjudge (keeps its database)
dj-down:
    {{dj}} --profile judge down

# Stop the local DOMjudge and delete its database
dj-reset:
    {{dj}} --profile judge down -v
    rm -f dev/domjudge/.env
