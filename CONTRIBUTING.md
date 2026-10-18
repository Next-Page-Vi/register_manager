# Contributing

Thank you for helping! Issues and pull requests are welcome.

## Ground rules

- Keep it simple. This project deliberately avoids generic frameworks,
  repositories, event buses and configurable permission systems.
- All booking rules live on the server, in `internal/booking` (pure engine in
  `engine.go`) and `internal/schedule`. The frontend never decides whether a
  time is bookable.
- Every write goes through `storage.DB.WriteTx` and re-validates permissions
  and rules inside the transaction.
- Every instance-scoped query includes `instance_id`.
- Interface text is never hard-coded: use `t('…')` (ESLint enforces it).
- Errors from the API carry a stable `code`; add a translation for any new
  code in every locale.
- Add tests: table tests for rules, `internal/server` HTTP tests for
  behaviour, Playwright for user flows. `web/e2e/layout.spec.ts` opens every
  page and dialog on small and low-density phone screens (down to 320×480);
  add new pages to it. Daily checks use four representative language/theme
  combinations; `pnpm -C web test:e2e:full` runs the full layout matrix.
- E2E projects cover desktop Chromium, Android Chromium and iPhone WebKit.
  Ordinary regressions run once on desktop. Tag core flows in `booking.spec.ts`
  with `@smoke` to repeat them on both phones. Tag rendering, native control,
  focus or navigation regressions with `@browser` to also run them on WebKit;
  avoid this tag for browser-independent data and request logic. Put desktop
  checks in `desktop.spec.ts`, and `@touch` checks in `mobile.spec.ts`.
  Select combinations in `playwright.config.ts` instead of collecting skipped tests.
- Form layout checks include each control's field boundary and date/time
  overflow. Exercise conditional fields too (e.g. turn off "Day off" in a
  special-date dialog). Screenshots are diagnostic artifacts, not visual
  assertions. Linux WebKit does not include the iOS native form theme; the
  date/time `appearance: none` guard covers its known sizing bug, but native
  pickers and their rendering still need a real iPhone check.
- The app must also work over plain HTTP (self-hosters often try it on a LAN
  or tailnet address), so don't rely on HTTPS-only browser APIs such as
  `crypto.randomUUID` or `navigator.clipboard` without a fallback (ESLint
  flags them).

Run before sending a pull request:

```sh
gofmt -l . && go vet ./... && go test ./...
pnpm -C web lint && pnpm -C web typecheck && pnpm -C web check:i18n && pnpm -C web check:api
```

## Changing the database or API

1. Add a new migration in `db/migrations/` (never edit an applied one).
   To change constraints, rebuild the table within the migration and preserve
   its data, indexes and triggers.
2. Update the database models in `internal/storage/models.go` and the Bun
   queries in the relevant service package. Share query helpers through
   `bun.IDB`; pass the current transaction to authorization and audit writes.
   Use pointers for nullable fields, explicit update columns, and parameterized
   values. Preserve Goose constraints and triggers; do not create tables from
   ORM models. Raw SQLite session and maintenance operations keep using
   `database/sql`.
3. After changing API types, run `pnpm -C web gen:api` and commit
   `web/src/api/openapi.json`, `schema.d.ts` and `constraints.ts`.
4. Policy numeric limits belong in `internal/booking/policy.go`; regenerate the
   API instead of maintaining a second set of frontend constants. Use
   `$api.useQuery/useInfiniteQuery/useMutation` and React Hook Form for new
   features. Keep request state out of shared dialog components.

## Adding a language

1. Copy `web/src/locales/en/translation.json` to
   `web/src/locales/<tag>/translation.json` (a BCP 47 tag such as `ko`,
   `de`, `fr`) and translate the values. Keep `{{placeholders}}`
   unchanged; use the plural suffixes your language needs (`_one`,
   `_other`, …).
2. Add the tag to `SUPPORTED_LOCALES` and its native name to `LOCALE_NAMES`
   in `web/src/i18n/index.ts`; extend `matchLocale` if browser tags need
   mapping.
3. Run `pnpm -C web check:i18n`.

No server or database change is needed.

## Licence

By contributing you agree that your contributions are licensed under the
MIT License.
