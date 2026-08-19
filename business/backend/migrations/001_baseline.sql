-- Module Registry schema baseline for MySQL 8.
CREATE TABLE IF NOT EXISTS platform_registry_state (
    id         SMALLINT    NOT NULL,
    revision   BIGINT      NOT NULL,
    releases   JSON        NOT NULL,
    active     JSON        NOT NULL,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    CONSTRAINT ck_registry_state_singleton CHECK (id = 1),
    CONSTRAINT ck_registry_state_revision CHECK (revision > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

INSERT INTO platform_registry_state (id, revision, releases, active)
VALUES (1, 1, JSON_OBJECT(), JSON_OBJECT())
ON DUPLICATE KEY UPDATE id = id;

CREATE TABLE IF NOT EXISTS platform_permission_catalog (
    module_id       VARCHAR(64)  COLLATE utf8mb4_0900_as_cs NOT NULL,
    permission_code VARCHAR(191) COLLATE utf8mb4_0900_as_cs NOT NULL,
    name            VARCHAR(191) NOT NULL,
    resource_code   VARCHAR(160) COLLATE utf8mb4_0900_as_cs NOT NULL,
    action          VARCHAR(30)  COLLATE utf8mb4_0900_as_cs NOT NULL,
    description     VARCHAR(512) NOT NULL DEFAULT '',
    active          TINYINT(1)   NOT NULL DEFAULT 1,
    release_version VARCHAR(32)  COLLATE utf8mb4_0900_as_cs NULL,
    updated_at      DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (permission_code),
    KEY idx_platform_permission_catalog_active (module_id, active, permission_code),
    CONSTRAINT ck_permission_code_shape CHECK (permission_code = CONCAT(resource_code, '.', action))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
