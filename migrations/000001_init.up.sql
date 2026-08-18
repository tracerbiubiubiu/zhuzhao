-- Phase 1 schema: users, roles, organizations, menus, associations, audit, casbin

CREATE EXTENSION IF NOT EXISTS ltree;

CREATE TABLE users (
    id                   BIGSERIAL PRIMARY KEY,
    username             VARCHAR(50) NOT NULL,
    employee_no          VARCHAR(50),
    domain_account       VARCHAR(100),
    user_domain          VARCHAR(255),
    password             VARCHAR(100) NOT NULL,
    real_name            VARCHAR(100),
    email                VARCHAR(100),
    phone                VARCHAR(20),
    avatar               VARCHAR(500),
    status               SMALLINT NOT NULL DEFAULT 1,
    must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
    last_login_at        TIMESTAMPTZ,
    last_login_ip        VARCHAR(50),
    is_system            BOOLEAN DEFAULT FALSE,
    tenant_id            BIGINT NOT NULL DEFAULT 1,
    version              INT DEFAULT 1,
    deleted_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ DEFAULT NOW(),
    updated_at           TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_users_username ON users(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_employee_no ON users(employee_no)
    WHERE employee_no IS NOT NULL AND employee_no <> '';
CREATE UNIQUE INDEX idx_users_domain_account ON users(user_domain, domain_account)
    WHERE domain_account IS NOT NULL AND domain_account <> ''
      AND user_domain IS NOT NULL AND user_domain <> '';
CREATE INDEX idx_users_tenant ON users(tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE roles (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    status      SMALLINT NOT NULL DEFAULT 1,
    priority    INT NOT NULL,
    sort_order  INT DEFAULT 0,
    is_system   BOOLEAN DEFAULT FALSE,
    tenant_id   BIGINT NOT NULL DEFAULT 1,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_roles_code ON roles(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_roles_tenant ON roles(tenant_id) WHERE deleted_at IS NULL;

CREATE TABLE organizations (
    id          BIGSERIAL PRIMARY KEY,
    code        VARCHAR(50) NOT NULL,
    name        VARCHAR(100) NOT NULL,
    description TEXT,
    parent_id   BIGINT REFERENCES organizations(id),
    path        LTREE NOT NULL,
    org_type    SMALLINT NOT NULL,
    status      SMALLINT NOT NULL DEFAULT 1,
    is_system   BOOLEAN DEFAULT FALSE,
    sort_order  INT DEFAULT 0,
    created_by  BIGINT,
    tenant_id   BIGINT NOT NULL DEFAULT 1,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_org_code ON organizations(code) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_path ON organizations USING GIST(path);
CREATE INDEX idx_org_parent ON organizations(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_org_deleted ON organizations(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TABLE menus (
    id          BIGSERIAL PRIMARY KEY,
    parent_id   BIGINT REFERENCES menus(id),
    code        VARCHAR(50) UNIQUE NOT NULL,
    name        VARCHAR(100) NOT NULL,
    menu_type   SMALLINT NOT NULL,
    path        VARCHAR(200),
    component   VARCHAR(200),
    icon        VARCHAR(100),
    permission  VARCHAR(100),
    sort_order  INT DEFAULT 0,
    visible     BOOLEAN DEFAULT TRUE,
    is_system   BOOLEAN DEFAULT FALSE,
    version     INT DEFAULT 1,
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    updated_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_menus_parent ON menus(parent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menus_deleted ON menus(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TABLE menu_apis (
    menu_id     BIGINT NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
    api_path    VARCHAR(200) NOT NULL,
    api_method  VARCHAR(10) NOT NULL,
    PRIMARY KEY (menu_id, api_path, api_method)
);

CREATE TABLE user_roles (
    user_id BIGINT NOT NULL REFERENCES users(id),
    role_id BIGINT NOT NULL REFERENCES roles(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(user_id, role_id)
);

CREATE TABLE user_orgs (
    user_id     BIGINT NOT NULL REFERENCES users(id),
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    is_primary  BOOLEAN DEFAULT FALSE,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(user_id, org_id)
);

CREATE TABLE role_menus (
    role_id BIGINT NOT NULL REFERENCES roles(id),
    menu_id BIGINT NOT NULL REFERENCES menus(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY(role_id, menu_id)
);

CREATE TABLE audit_logs (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT,
    username     VARCHAR(100),
    method       VARCHAR(10) NOT NULL,
    path         VARCHAR(500) NOT NULL,
    status_code  INT NOT NULL,
    duration     BIGINT NOT NULL,
    ip           VARCHAR(50),
    user_agent   VARCHAR(500),
    request_body TEXT,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_user_time ON audit_logs(user_id, created_at DESC);
CREATE INDEX idx_audit_path_time ON audit_logs(path, created_at DESC);

CREATE TABLE casbin_rule (
    id    BIGSERIAL PRIMARY KEY,
    p_type VARCHAR(10) NOT NULL,
    v0    VARCHAR(255) NOT NULL,
    v1    VARCHAR(255) NOT NULL,
    v2    VARCHAR(255) DEFAULT '',
    v3    VARCHAR(255) DEFAULT '',
    v4    VARCHAR(255) DEFAULT '',
    v5    VARCHAR(255) DEFAULT ''
);

CREATE UNIQUE INDEX idx_casbin_rule ON casbin_rule (p_type, v0, v1, v2);
