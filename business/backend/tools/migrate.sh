#!/bin/sh
set -eu

mysql_cmd="mysql --default-character-set=utf8mb4 -h ${MYSQL_HOST:?MYSQL_HOST is required} -u${MYSQL_USER:?MYSQL_USER is required} ${MYSQL_DATABASE:?MYSQL_DATABASE is required}"
$mysql_cmd -e "CREATE TABLE IF NOT EXISTS platform_schema_migration(filename VARCHAR(191) COLLATE utf8mb4_0900_as_cs NOT NULL, sha256 CHAR(64) COLLATE utf8mb4_0900_as_cs NOT NULL, applied_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), PRIMARY KEY(filename)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci"

for file in /migrations/*.sql; do
  filename=$(basename "$file")
  digest=$(sha256sum "$file" | cut -d' ' -f1)
  applied=$($mysql_cmd -Nse "SELECT sha256 FROM platform_schema_migration WHERE filename='${filename}'")
  if [ -n "$applied" ]; then
    if [ "$applied" != "$digest" ]; then
      echo "migration checksum changed: $filename" >&2
      exit 1
    fi
    continue
  fi
  $mysql_cmd < "$file"
  $mysql_cmd -e "INSERT INTO platform_schema_migration(filename,sha256) VALUES('${filename}','${digest}')"
done
