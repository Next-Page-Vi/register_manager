# register_manager

A simple, self-hosted appointment booking system for people who each run
their own schedule — a doctor, a therapist, a hairdresser — designed so that
elderly customers can book in three steps: **open link → pick a time →
confirm**.

- **One binary, one file.** A Go server with the web app embedded, and a
  single SQLite database file. No other services to run.
- **Many independent calendars.** Each calendar (an *instance*) has its own
  customers, working hours, rules, time zone, language and registration QR
  code; a person who books with two calendars is two unrelated customers.
  Operators can be given any number of calendars, and the super admin can
  create and cleanly delete calendars at any time. Data never leaks between
  calendars.
- **Flexible working hours.** Repeating cycles of any length (not just
  weeks), special dates, blocked times, lunch breaks, buffers between
  appointments, per-customer daily/weekly limits, booking horizon and notice.
- **Global.** Every calendar has an IANA time zone; daylight-saving
  transitions are handled explicitly. The interface is available in
  English, 日本語, 简体中文 and 繁體中文. Add another language with translation
  files and locale registration.
- **No passwords for customers.** Customers register from a QR code, staff
  approve them and hand over a personal link. Links can be revoked at any
  time.
- **Reliable.** Double-clicks, retries and the back button never create
  duplicate bookings; two people can never book the same time.

Licensed under the [MIT License](LICENSE).

## Quick start

Requirements: Go 1.27.2+, Node.js 26+ with pnpm 12.11.2 (only for building the web
app).

```sh
pnpm -C web install
pnpm -C web build          # builds web/dist, embedded into the server
go build -o bin/server ./cmd/server

# Run. Behind HTTPS in production; for local testing over plain HTTP:
GOMEMLIMIT=96MiB RM_DB_PATH=data/app.db RM_COOKIE_SECURE=false ./bin/server
```

On the first start the server creates the one super admin, login name
`admin`, and prints its generated password once to standard error. Open
<http://localhost:8080/account/login>, sign in and choose a new password,
then:

1. **Accounts → New operator** for each person who runs a calendar.
2. **All calendars → New calendar** with its time zone and default language
   (optionally copying the settings of another calendar), then **Add
   operator**. New calendars are paused.
3. The operator signs in, sets **Working hours** and **Booking rules**, then
   **Settings → Open for booking** and prints the registration QR code.
4. Customers scan the QR code and register; the operator approves them under
   **Customers** and hands over the personal link shown (once) as a QR code.

There is exactly one super admin; it cannot be created, deleted or disabled.
Every account changes its own login name and password under **My account**.

### Docker

```sh
docker compose -f deploy/compose.yaml up -d
docker compose -f deploy/compose.yaml logs app   # shows the super admin's first password
```

## Configuration

All configuration is through environment variables.

| Variable | Default | Meaning |
| --- | --- | --- |
| `RM_DB_PATH` | `data/register_manager.db` | SQLite database file. Must be on a local disk, never a network file system. |
| `RM_LISTEN` | `:8080` | HTTP listen address. |
| `RM_TRUSTED_PROXY_HEADER` | *(empty)* | Header your reverse proxy sets with the client address (`X-Forwarded-For` or `X-Real-IP`), used for rate limiting. Leave empty if there is no proxy. |
| `RM_COOKIE_SECURE` | `true` | Mark cookies `Secure`. Set to `false` only for local HTTP testing. |
| `RM_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `GOMEMLIMIT` | `96MiB` in Docker and the commands above | Go's soft runtime memory limit, not a hard RSS cap. Override for your workload; a bare binary uses Go's default when unset. |

HTTPS, certificates and the reverse proxy are up to you (Caddy, nginx,
Traefik, …). The server is single-node by design.

Passwords use Argon2id with 19 MiB of memory, two passes, one lane, a random
16-byte salt and a 32-byte key, following an
[OWASP recommended profile](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#argon2id).
All password operations share a limit of two concurrent computations. Requests
can cancel while waiting; a running hash completes normally. Existing accounts
are checked before hashing on startup, and unknown accounts use a precomputed
dummy hash for equivalent verification work.

This pre-release only accepts the current password profile. Databases from the
previous 64 MiB profile must be recreated; there is no password migration.

## Administration

Each calendar's **Statistics** page shows appointment totals, distinct customers,
trends and current status counts. Choose appointment start dates or creation dates
and a range of up to 366 days. Dates use the calendar's current time zone; trends
group by day (up to 31 days), Monday-based week (up to 180 days), or calendar month.
The totals include cancelled appointments and their customers. Statuses reflect
the current records, so rescheduling and status corrections update past reports.
Past appointments are never automatically marked completed or no-show.

```sh
server admin reset-password                  # set the super admin's password; prints its login name
server backup --to /backups/app-2026-10-14.db
```

Passwords are read from the terminal (or one line on standard input), never
from arguments. `admin reset-password` is the recovery path if the super
admin's password (or login name) is lost; run before the first start, it
creates the super admin with that password instead of a generated one.

Deleting a calendar removes its data from the database file (SQLite
`secure_delete`), but existing backups and Litestream replicas keep it until
their retention period ends; shorten the retention if deletions must reach
backups sooner.

### Backups

Do not copy the database file while the server runs. Use `server backup`
(a consistent `VACUUM INTO` snapshot) on a schedule and copy the result off
the machine, or run [Litestream](https://litestream.io) for continuous
replication (example in `deploy/litestream.yml`). To restore, stop the
server, replace the database file and start it again. The server also
writes a backup automatically before applying database migrations.

## Development

```sh
GOMEMLIMIT=96MiB RM_COOKIE_SECURE=false go run ./cmd/server # API on :8080
pnpm -C web dev                # Vite dev server on :5173, proxies the API
```

| Task | Command |
| --- | --- |
| Go tests (engine, DST, concurrency, isolation, HTTP) | `go test ./...` |
| Password CPU and allocation benchmarks | `GOMEMLIMIT=96MiB go test ./internal/auth -run '^$' -bench Password -benchmem` |
| Fuzz the booking engine | `go test ./internal/booking -run '^$' -fuzz FuzzSlotsAreValid` |
| Database query benchmarks | `GOMEMLIMIT=96MiB go test ./internal/server -run '^$' -bench BenchmarkLists -benchmem` |
| Regenerate API types and policy constraints | `pnpm -C web gen:api` |
| Frontend lint / types / translations | `pnpm -C web lint && pnpm -C web typecheck && pnpm -C web check:i18n` |
| Install end-to-end browsers and system dependencies | `pnpm -C web exec playwright install --with-deps chromium webkit` |
| Daily end-to-end tests (also used by CI) | `pnpm -C web build && pnpm -C web test:e2e` |
| Full layout matrix (after building; before releases or broad UI changes) | `pnpm -C web test:e2e:full` |
| iPhone flows and representative layouts (after building) | `pnpm -C web test:e2e --project=iphone --project=layout-iphone-small` |

End-to-end tests start a real Go server with a throwaway SQLite database.
The `desktop` project uses Chromium, `mobile` uses Pixel 7 emulation, and
`iphone` uses WebKit with the iPhone 16 preset. Ordinary regressions run once
on desktop; phones repeat core booking/schedule flows and touch interactions,
and iPhone also runs browser-specific regressions.

Daily layout checks cover every page and dialog in four combinations:
320×480 low-density Chromium in English/light and Simplified Chinese/dark;
375×667 WebKit in Japanese/light and Traditional Chinese/dark.
`test:e2e:full` keeps these daily checks and adds all four languages and both
themes on Pixel 7 and iPhone 16, plus all languages in light mode at both small sizes.
It keeps the same functional tests instead of repeating ordinary regressions.
Failures retain traces and screenshots in `web/test-results`, uploaded by CI.
Desktop density checks cover 768–1920px layouts, while shared checks verify
date navigation, portal sizing, validation and enlarged text. See
[DESIGN.md](DESIGN.md) for the staff/customer scales and layout rules.
Device emulation does not verify a particular iOS release: use a real iPhone
for Safari/WeChat, the software keyboard, browser chrome and native pickers.

The frontend uses TanStack Query through openapi-react-query for server state,
and React Hook Form for editable drafts. Go policy constraints generate both
OpenAPI and the frontend limits; do not edit the generated files by hand.

The backend uses Bun with the pure-Go SQLite driver and Goose SQL migrations.
Query helpers accept `bun.IDB` so the same query can run in a transaction.

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to contribute, including adding
a language.
