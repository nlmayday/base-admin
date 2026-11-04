-- MySQL version of ../000001_auth_version.up.sql. Every table of the MySQL
-- schema uses InnoDB and the case-sensitive utf8mb4_0900_as_cs collation, so
-- comparisons and unique keys behave as on PostgreSQL.
CREATE TABLE sys_auth_version (
    id      smallint PRIMARY KEY CHECK (id = 1),
    version bigint   NOT NULL DEFAULT 1 CHECK (version >= 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;

INSERT INTO sys_auth_version (id, version) VALUES (1, 1);

-- MySQL only: rows locked by database.LockTx. MySQL named locks
-- (GET_LOCK) outlive transactions; a locked row is released at COMMIT or
-- ROLLBACK, like pg_advisory_xact_lock on PostgreSQL.
CREATE TABLE sys_tx_locks (
    name varchar(128) PRIMARY KEY
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
