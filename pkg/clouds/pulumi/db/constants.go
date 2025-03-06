package db

const PSQL_DB_INIT_SH = `
set -e;
apk add --no-cache postgresql-client; 
psql -tc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1 || psql -c "CREATE DATABASE \"$DB_NAME\"";
psql -c "DO
\$\$
BEGIN
  IF NOT EXISTS (SELECT * FROM pg_user WHERE usename = '$DB_USER') THEN
	CREATE ROLE \"$DB_USER\" WITH LOGIN PASSWORD '$DB_PASSWORD'; 
	GRANT ALL PRIVILEGES ON DATABASE \"$DB_NAME\" TO \"$DB_USER\";
	ALTER DATABASE \"$DB_NAME\" OWNER TO \"$DB_USER\";
	$INIT_SQL
  END IF;
END
\$\$
;"
`
