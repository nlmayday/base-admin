-- MySQL version of ../000009_ops.up.sql. trigger is a reserved word in
-- MySQL; the application's SQL quotes it.

CREATE TABLE sys_jobs (
    id              bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name            varchar(64)  NOT NULL,
    -- Key of a handler registered in code through jobs.Provider.
    handler_key     varchar(64)  NOT NULL,
    -- robfig/cron v3 expression: 5 standard fields or a descriptor (@daily, @every 5m).
    cron_expr       varchar(128) NOT NULL,
    params          json         NOT NULL DEFAULT (JSON_OBJECT()),
    enabled         boolean      NOT NULL DEFAULT true,
    -- 0 means the handler's default timeout.
    timeout_seconds integer      NOT NULL DEFAULT 0 CHECK (timeout_seconds BETWEEN 0 AND 86400),
    max_retries     integer      NOT NULL DEFAULT 0 CHECK (max_retries BETWEEN 0 AND 10),
    remark          varchar(255) NOT NULL DEFAULT '',
    version         integer      NOT NULL DEFAULT 0,
    created_at      datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at      datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_by      bigint,
    updated_by      bigint,
    last_run_at     datetime(6),
    last_status     varchar(16)  NOT NULL DEFAULT ''
        CHECK (last_status IN ('', 'running', 'success', 'failed', 'timeout', 'skipped'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE UNIQUE INDEX sys_jobs_name_uq ON sys_jobs (name);

-- One row per fire (cron) or per manual trigger. Retries of the same fire
-- update the row: attempt is the number of attempts made so far.
CREATE TABLE sys_job_runs (
    id           bigint       NOT NULL AUTO_INCREMENT PRIMARY KEY,
    -- Kept when the job is deleted so the history survives.
    job_id       bigint,
    job_name     varchar(64)  NOT NULL,
    handler_key  varchar(64)  NOT NULL,
    `trigger`    varchar(8)   NOT NULL CHECK (`trigger` IN ('cron', 'manual')),
    status       varchar(16)  NOT NULL
        CHECK (status IN ('running', 'success', 'failed', 'timeout', 'skipped')),
    attempt      integer      NOT NULL DEFAULT 1,
    -- The cron fire time; null for manual runs.
    scheduled_at datetime(6),
    started_at   datetime(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    finished_at  datetime(6),
    duration_ms  bigint,
    error        text         NOT NULL DEFAULT (''),
    instance     varchar(128) NOT NULL DEFAULT '',
    triggered_by bigint,
    -- A run still 'running' after this time lost its instance and is marked failed.
    expires_at   datetime(6)  NOT NULL,
    CONSTRAINT sys_job_runs_job_fk FOREIGN KEY (job_id) REFERENCES sys_jobs (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_as_cs;
CREATE INDEX sys_job_runs_started_at_idx ON sys_job_runs (started_at DESC);
CREATE INDEX sys_job_runs_job_id_idx ON sys_job_runs (job_id, started_at DESC);
CREATE INDEX sys_job_runs_running_idx ON sys_job_runs (status, expires_at);

-- Built-in retention job; inserted once so administrators may delete it.
INSERT INTO sys_jobs (name, handler_key, cron_expr, params, enabled, remark)
VALUES ('清理执行记录', 'ops.cleanup-runs', '0 3 * * *', '{"days": 30}', true, '每天凌晨 3 点删除 30 天前的执行记录');
