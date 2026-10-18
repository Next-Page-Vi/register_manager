// Verifies that every locale has exactly the same translation keys as the
// English base, and that interpolation placeholders match.
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const dir = join(dirname(fileURLToPath(import.meta.url)), '../src/locales')
const base = 'en'

function flatten(obj, prefix = '', out = {}) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object') flatten(v, key, out)
    else out[key] = String(v)
  }
  return out
}

// Plural suffixes differ by language (English has _one/_other, Japanese and
// Chinese only _other); compare keys with plural suffixes stripped.
const pluralSuffix = /_(zero|one|two|few|many|other)$/
const stem = (k) => k.replace(pluralSuffix, '')
const placeholders = (s) => [...s.matchAll(/\{\{\s*([\w.]+)[^}]*\}\}/g)].map((m) => m[1]).sort().join(',')

const load = (lng) => flatten(JSON.parse(readFileSync(join(dir, lng, 'translation.json'), 'utf8')))
const ref = load(base)
const refStems = new Map()
for (const [k, v] of Object.entries(ref)) refStems.set(stem(k), placeholders(v))

let failed = false
for (const lng of readdirSync(dir)) {
  if (lng === base) continue
  const cur = load(lng)
  const stems = new Map()
  for (const [k, v] of Object.entries(cur)) stems.set(stem(k), placeholders(v))
  for (const [k, ph] of refStems) {
    if (!stems.has(k)) {
      console.error(`[${lng}] missing key: ${k}`)
      failed = true
    } else if (stems.get(k) !== ph) {
      console.error(`[${lng}] placeholder mismatch in ${k}: "${stems.get(k)}" vs "${ph}"`)
      failed = true
    }
  }
  for (const k of stems.keys()) {
    if (!refStems.has(k)) {
      console.error(`[${lng}] extra key: ${k}`)
      failed = true
    }
  }
}
if (failed) process.exit(1)
console.log('Translations are complete.')
