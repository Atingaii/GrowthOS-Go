#!/bin/sh
set -eu

secret_file=/run/secrets/redis_password
runtime_dir=/tmp/growthos-redis

if [ ! -r "$secret_file" ]; then
    printf '%s\n' 'redis password secret is unavailable' >&2
    exit 1
fi

password=$(tr -d '\r\n' < "$secret_file")
case "$password" in
    ''|*[!0-9a-f]*)
        printf '%s\n' 'redis password secret must be lowercase hexadecimal' >&2
        exit 1
        ;;
esac
if [ "${#password}" -ne 64 ]; then
    printf '%s\n' 'redis password secret must contain exactly 64 hexadecimal characters' >&2
    exit 1
fi

umask 077
mkdir -p "$runtime_dir"
{
    printf '%s\n' 'user default off'
    printf 'user growthos_api on >%s ' "$password"
    printf '%s ' '~growthos:development:lottery:strategy:projection:v1:*'
    printf '%s\n' '&* +ping +getrange +set +del'
} > "$runtime_dir/users.acl"
cat > "$runtime_dir/redis.conf" <<'EOF'
bind 0.0.0.0
protected-mode yes
port 6379
dir /data
save ""
appendonly no
maxmemory 48mb
maxmemory-policy allkeys-lru
aclfile /tmp/growthos-redis/users.acl
EOF

unset password
exec "$@"
