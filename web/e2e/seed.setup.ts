import { test as setup } from '@playwright/test'
import { writeFileSync } from 'node:fs'
import { SEED_FILE, seed } from './seed'

setup('seed layout data', async ({ playwright, baseURL }) => {
  writeFileSync(SEED_FILE, JSON.stringify(await seed(playwright, baseURL!, Date.now().toString(36))))
})
