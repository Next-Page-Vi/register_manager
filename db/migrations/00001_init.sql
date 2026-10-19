-- +goose Up

-- Times are UTC Unix milliseconds (INTEGER, *_at columns).
-- Local dates are TEXT 'YYYY-MM-DD'. IDs are lowercase UUID text.

CREATE TABLE accounts (
  id                   TEXT    PRIMARY KEY,
  login                TEXT    NOT NULL UNIQUE CHECK (length(login) BETWEEN 1 AND 64),
  name                 TEXT    NOT NULL CHECK (length(trim(name)) > 0),
  password_hash        TEXT    NOT NULL,
  auth_version         INTEGER NOT NULL DEFAULT 1,
  disabled_at          INTEGER,
  locale               TEXT,
  super_admin          INTEGER NOT NULL DEFAULT 0 CHECK (super_admin IN (0, 1)),
  must_change_password INTEGER NOT NULL DEFAULT 0 CHECK (must_change_password IN (0, 1)),
  created_at           INTEGER NOT NULL,
  updated_at           INTEGER NOT NULL,
  -- The super admin can never be disabled.
  CHECK (super_admin = 0 OR disabled_at IS NULL)
) STRICT;

-- There is exactly one super admin, created on first start (see
-- internal/accounts); this index makes a second one impossible.
CREATE UNIQUE INDEX accounts_one_super_admin ON accounts (super_admin) WHERE super_admin = 1;

-- System-wide settings, managed by the super admin. One row.
CREATE TABLE platform_settings (
  id             INTEGER PRIMARY KEY CHECK (id = 1),
  -- Longest repeating schedule an instance may save. The hard limit stays
  -- 366 (schedules.cycle_length_days); lowering this keeps existing longer
  -- schedules working until they are next saved.
  max_cycle_days INTEGER NOT NULL DEFAULT 30 CHECK (max_cycle_days BETWEEN 1 AND 366),
  updated_at     INTEGER NOT NULL
) STRICT;

INSERT INTO platform_settings (id, max_cycle_days, updated_at) VALUES (1, 30, 0);

CREATE TABLE instances (
  id                       TEXT    PRIMARY KEY,
  slug                     TEXT    NOT NULL UNIQUE CHECK (length(slug) BETWEEN 2 AND 40),
  name                     TEXT    NOT NULL CHECK (length(trim(name)) > 0),
  status                   TEXT    NOT NULL CHECK (status IN ('active', 'paused')),
  timezone                 TEXT    NOT NULL,
  default_locale           TEXT    NOT NULL,
  registration_invite_code TEXT    NOT NULL,
  contact_kind             TEXT    NOT NULL DEFAULT 'phone'
    CHECK (contact_kind IN ('phone', 'email', 'text')),
  contact_required         INTEGER NOT NULL DEFAULT 1 CHECK (contact_required IN (0, 1)),
  -- Optional colour for the top of customer pages, lowercase "#rrggbb".
  brand_color              TEXT
    CHECK (brand_color IS NULL OR brand_color GLOB '#[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]'),
  created_at               INTEGER NOT NULL,
  updated_at               INTEGER NOT NULL
) STRICT;

-- Operators bound to an instance (many-to-many). The super admin is never
-- bound; it reaches every instance without a binding.
CREATE TABLE instance_operators (
  instance_id TEXT    NOT NULL REFERENCES instances(id),
  account_id  TEXT    NOT NULL REFERENCES accounts(id),
  created_at  INTEGER NOT NULL,
  PRIMARY KEY (instance_id, account_id)
) STRICT;

CREATE INDEX instance_operators_account ON instance_operators (account_id);

CREATE TABLE booking_policies (
  instance_id                     TEXT    PRIMARY KEY REFERENCES instances(id),
  slot_step_minutes               INTEGER NOT NULL CHECK (slot_step_minutes BETWEEN 1 AND 1440),
  service_duration_minutes        INTEGER NOT NULL CHECK (service_duration_minutes BETWEEN 1 AND 1440),
  buffer_minutes                  INTEGER NOT NULL CHECK (buffer_minutes BETWEEN 0 AND 1440),
  daily_customer_limit            INTEGER CHECK (daily_customer_limit IS NULL OR daily_customer_limit BETWEEN 0 AND 10000),
  per_customer_daily_limit        INTEGER CHECK (per_customer_daily_limit IS NULL OR per_customer_daily_limit BETWEEN 0 AND 10000),
  per_customer_weekly_limit       INTEGER CHECK (per_customer_weekly_limit IS NULL OR per_customer_weekly_limit BETWEEN 0 AND 10000),
  max_future_bookings             INTEGER CHECK (max_future_bookings IS NULL OR max_future_bookings BETWEEN 0 AND 10000),
  booking_horizon_days            INTEGER NOT NULL CHECK (booking_horizon_days BETWEEN 0 AND 366),
  min_booking_notice_minutes      INTEGER NOT NULL CHECK (min_booking_notice_minutes BETWEEN 0 AND 525600),
  min_cancellation_notice_minutes INTEGER NOT NULL CHECK (min_cancellation_notice_minutes BETWEEN 0 AND 525600),
  updated_at                      INTEGER NOT NULL
) STRICT;

CREATE TABLE schedules (
  instance_id       TEXT    PRIMARY KEY REFERENCES instances(id),
  cycle_length_days INTEGER NOT NULL CHECK (cycle_length_days BETWEEN 1 AND 366),
  anchor_date       TEXT    NOT NULL CHECK (date(anchor_date) IS anchor_date),
  cycle_days        TEXT    NOT NULL CHECK (
    json_valid(cycle_days)
    AND json_type(cycle_days) = 'array'
    AND json_array_length(cycle_days) = cycle_length_days
  ),
  updated_at        INTEGER NOT NULL
) STRICT;

CREATE TABLE schedule_overrides (
  instance_id TEXT    NOT NULL REFERENCES instances(id),
  date        TEXT    NOT NULL CHECK (date(date) IS date),
  periods     TEXT    NOT NULL CHECK (json_valid(periods) AND json_type(periods) = 'array'),
  updated_at  INTEGER NOT NULL,
  PRIMARY KEY (instance_id, date)
) STRICT;

CREATE TABLE blocked_periods (
  id          TEXT    PRIMARY KEY,
  instance_id TEXT    NOT NULL REFERENCES instances(id),
  starts_at   INTEGER NOT NULL,
  ends_at     INTEGER NOT NULL,
  reason      TEXT,
  created_at  INTEGER NOT NULL,
  CHECK (starts_at < ends_at)
) STRICT;

CREATE INDEX blocked_periods_instance_end ON blocked_periods (instance_id, ends_at);

CREATE TABLE customers (
  id                  TEXT    PRIMARY KEY,
  instance_id         TEXT    NOT NULL REFERENCES instances(id),
  name                TEXT    NOT NULL CHECK (length(trim(name)) > 0),
  contact             TEXT,
  status              TEXT    NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'disabled')),
  internal_note       TEXT,
  locale              TEXT,
  access_token_digest TEXT    UNIQUE,
  auth_version        INTEGER NOT NULL DEFAULT 1,
  created_at          INTEGER NOT NULL,
  updated_at          INTEGER NOT NULL,
  UNIQUE (instance_id, id),
  CHECK ((status = 'approved') = (access_token_digest IS NOT NULL))
) STRICT;

CREATE INDEX customers_instance_status ON customers (instance_id, status, created_at);

CREATE TABLE appointments (
  id           TEXT    PRIMARY KEY,
  instance_id  TEXT    NOT NULL REFERENCES instances(id),
  customer_id  TEXT    NOT NULL,
  starts_at    INTEGER NOT NULL,
  ends_at      INTEGER NOT NULL,
  status       TEXT    NOT NULL DEFAULT 'booked'
    CHECK (status IN ('booked', 'cancelled', 'completed', 'no_show')),
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  UNIQUE (instance_id, id),
  FOREIGN KEY (instance_id, customer_id) REFERENCES customers (instance_id, id),
  CHECK (starts_at < ends_at)
) STRICT;

CREATE INDEX appointments_instance_start ON appointments (instance_id, starts_at);
CREATE INDEX appointments_instance_created ON appointments (instance_id, created_at);
CREATE INDEX appointments_customer_start ON appointments (instance_id, customer_id, starts_at);
CREATE INDEX appointments_occupying ON appointments (instance_id, ends_at) WHERE status <> 'cancelled';

-- Last line of defence against overlapping service time within one instance.
-- Business checks run first inside the serialized write transaction.
-- +goose StatementBegin
CREATE TRIGGER appointments_no_overlap_insert
BEFORE INSERT ON appointments
WHEN NEW.status <> 'cancelled'
BEGIN
  SELECT RAISE(ABORT, 'appointment_overlap')
  WHERE EXISTS (
    SELECT 1 FROM appointments
    WHERE instance_id = NEW.instance_id
      AND status <> 'cancelled'
      AND ends_at > NEW.starts_at
      AND starts_at < NEW.ends_at
  );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER appointments_no_overlap_update
BEFORE UPDATE OF instance_id, starts_at, ends_at, status ON appointments
WHEN NEW.status <> 'cancelled'
BEGIN
  SELECT RAISE(ABORT, 'appointment_overlap')
  WHERE EXISTS (
    SELECT 1 FROM appointments
    WHERE instance_id = NEW.instance_id
      AND id <> NEW.id
      AND status <> 'cancelled'
      AND ends_at > NEW.starts_at
      AND starts_at < NEW.ends_at
  );
END;
-- +goose StatementEnd

CREATE TABLE audit_log (
  id                INTEGER PRIMARY KEY AUTOINCREMENT,
  at                INTEGER NOT NULL,
  scope             TEXT    NOT NULL CHECK (scope IN ('platform', 'instance')),
  instance_id       TEXT    REFERENCES instances(id),
  actor_kind        TEXT    NOT NULL CHECK (actor_kind IN ('account', 'customer', 'cli')),
  actor_account_id  TEXT    REFERENCES accounts(id),
  actor_customer_id TEXT    REFERENCES customers(id),
  action            TEXT    NOT NULL,
  target_type       TEXT,
  target_id         TEXT,
  before_json       TEXT    CHECK (before_json IS NULL OR json_valid(before_json)),
  after_json        TEXT    CHECK (after_json IS NULL OR json_valid(after_json)),
  CHECK ((scope = 'platform') = (instance_id IS NULL)),
  CHECK ((actor_kind = 'account') = (actor_account_id IS NOT NULL)),
  CHECK ((actor_kind = 'customer') = (actor_customer_id IS NOT NULL))
) STRICT;

CREATE INDEX audit_log_instance ON audit_log (instance_id, id);
-- Deleting a customer or account checks these references; without the
-- indexes each deleted row would scan the whole log.
CREATE INDEX audit_log_actor_customer ON audit_log (actor_customer_id) WHERE actor_customer_id IS NOT NULL;
CREATE INDEX audit_log_actor_account ON audit_log (actor_account_id) WHERE actor_account_id IS NOT NULL;

CREATE TABLE account_sessions (
  token  TEXT    PRIMARY KEY,
  data   BLOB    NOT NULL,
  expiry INTEGER NOT NULL
) STRICT;

CREATE INDEX account_sessions_expiry ON account_sessions (expiry);

CREATE TABLE customer_sessions (
  token  TEXT    PRIMARY KEY,
  data   BLOB    NOT NULL,
  expiry INTEGER NOT NULL
) STRICT;

CREATE INDEX customer_sessions_expiry ON customer_sessions (expiry);

-- +goose Down
DROP TABLE customer_sessions;
DROP TABLE account_sessions;
DROP INDEX audit_log_actor_account;
DROP INDEX audit_log_actor_customer;
DROP TABLE audit_log;
DROP TRIGGER appointments_no_overlap_update;
DROP TRIGGER appointments_no_overlap_insert;
DROP TABLE appointments;
DROP TABLE customers;
DROP TABLE blocked_periods;
DROP TABLE schedule_overrides;
DROP TABLE schedules;
DROP TABLE booking_policies;
DROP TABLE instance_operators;
DROP TABLE instances;
DROP TABLE platform_settings;
DROP INDEX accounts_one_super_admin;
DROP TABLE accounts;
