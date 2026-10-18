import { defineConfig, devices } from '@playwright/test'

const fullLayout = process.env.E2E_FULL_LAYOUT === '1'

// End-to-end tests run against the real Go server with a fresh database.
// Build the frontend first (`pnpm build`); e2e/server.mjs builds the server.
export default defineConfig({
  testDir: 'e2e',
  timeout: 90_000,
  // Tests are independent (each flow uses its own calendar; the layout data
  // is seeded once and only read), so they run in parallel on half the CPUs.
  fullyParallel: true,
  reporter: [['list']],
  use: {
    baseURL: 'http://127.0.0.1:18080',
    timezoneId: 'Europe/London',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    // Run every regression once. Phones repeat core flows and browser-specific checks.
    { name: 'desktop', testMatch: ['booking.spec.ts', 'statistics.spec.ts', 'regressions.spec.ts', 'design-fixes.spec.ts', 'desktop.spec.ts', 'density.spec.ts'], dependencies: ['seed'], use: { ...devices['Desktop Chrome'], locale: 'en-GB' } },
    { name: 'mobile', testMatch: ['booking.spec.ts', 'mobile.spec.ts'], grep: /@smoke|@touch/, dependencies: ['seed'], use: { ...devices['Pixel 7'], locale: 'en-GB' } },
    { name: 'iphone', testMatch: ['booking.spec.ts', 'statistics.spec.ts', 'regressions.spec.ts', 'design-fixes.spec.ts', 'mobile.spec.ts', 'density.spec.ts'], grep: /@smoke|@touch|@browser/, dependencies: ['seed'], use: { ...devices['iPhone 16'], browserName: 'webkit', locale: 'en-GB' } },
    // Creates the data the layout checks read.
    { name: 'seed', testMatch: 'seed.setup.ts' },
    // The full layout matrix adds standard phones in every language and theme.
    ...(fullLayout ? [
      { name: 'layout-pixel7', testMatch: 'layout.spec.ts', dependencies: ['seed'], use: { ...devices['Pixel 7'], locale: 'en-GB' } },
      { name: 'layout-iphone', testMatch: 'layout.spec.ts', dependencies: ['seed'], use: { ...devices['iPhone 16'], browserName: 'webkit' as const, locale: 'en-GB' } },
    ] : []),
    // Daily checks cover every page in four representative combinations:
    // both engines and themes, all languages, and both small-screen boundaries.
    // Full runs add all languages in light mode, retaining the daily dark checks.
    {
      name: 'layout-iphone-small', testMatch: 'layout.spec.ts', dependencies: ['seed'],
      grep: fullLayout ? /, light|zh-Hant, dark/ : /ja, light|zh-Hant, dark/,
      use: {
        ...devices['iPhone 16'], browserName: 'webkit', locale: 'en-GB',
        viewport: { width: 375, height: 667 }, screen: { width: 375, height: 667 }, deviceScaleFactor: 2,
      },
    },
    {
      name: 'layout-320x480-ldpi', testMatch: 'layout.spec.ts', dependencies: ['seed'],
      grep: fullLayout ? /, light|zh-Hans, dark/ : /en, light|zh-Hans, dark/,
      use: {
        ...devices['Pixel 7'], locale: 'en-GB',
        viewport: { width: 320, height: 480 }, screen: { width: 320, height: 480 }, deviceScaleFactor: 1,
      },
    },
  ],
  webServer: {
    command: 'node e2e/server.mjs',
    url: 'http://127.0.0.1:18080/healthz',
    reuseExistingServer: false,
    timeout: 240_000,
  },
})
