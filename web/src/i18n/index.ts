import i18n from 'i18next'
import resourcesToBackend from 'i18next-resources-to-backend'
import { initReactI18next } from 'react-i18next'
import type en from '@/locales/en/translation.json'

/**
 * Supported interface languages. Adding a language means adding
 * src/locales/<tag>/translation.json and listing the tag here; the server
 * needs no change.
 */
export const SUPPORTED_LOCALES = ['en', 'ja', 'zh-Hans', 'zh-Hant'] as const
export type Locale = (typeof SUPPORTED_LOCALES)[number]

/** Native language names, shown in the language switcher. */
export const LOCALE_NAMES: Record<Locale, string> = {
  en: 'English',
  ja: '日本語',
  'zh-Hans': '简体中文',
  'zh-Hant': '繁體中文',
}

const STORAGE_KEY = 'rm_locale'

/** Maps any BCP 47 tag to a supported locale, if possible. */
export function matchLocale(tag: string | null | undefined): Locale | undefined {
  if (!tag) return undefined
  const t = tag.toLowerCase()
  for (const l of SUPPORTED_LOCALES) if (l.toLowerCase() === t) return l
  // Traditional script, or a region that writes it; other Chinese is Simplified.
  if (/^zh-(hant|tw|hk|mo)(-|$)/.test(t)) return 'zh-Hant'
  if (t === 'zh' || t.startsWith('zh-')) return 'zh-Hans'
  if (t.startsWith('ja')) return 'ja'
  if (t.startsWith('en')) return 'en'
  return undefined
}

export function storedLocale(): Locale | undefined {
  try {
    return matchLocale(localStorage.getItem(STORAGE_KEY))
  } catch {
    return undefined
  }
}

export function storeLocale(l: Locale) {
  try {
    localStorage.setItem(STORAGE_KEY, l)
  } catch {
    // Private mode or blocked storage: the choice lasts for this page only.
  }
}

function browserLocale(): Locale | undefined {
  for (const tag of navigator.languages ?? [navigator.language]) {
    const l = matchLocale(tag)
    if (l) return l
  }
  return undefined
}

/**
 * Applies the language priority: the signed-in user's saved locale, then
 * the choice remembered in this browser, then the instance default, then
 * the browser language, then English.
 */
export function applyLocale(opts: { profile?: string | null; instanceDefault?: string | null }) {
  const l =
    matchLocale(opts.profile) ?? storedLocale() ?? matchLocale(opts.instanceDefault) ?? browserLocale() ?? 'en'
  if (i18n.language !== l) void i18n.changeLanguage(l)
}

export function currentLocale(): Locale {
  return matchLocale(i18n.language) ?? 'en'
}

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: { translation: typeof en }
  }
}

void i18n
  .use(
    resourcesToBackend((lng: string) => import(`../locales/${lng}/translation.json`)),
  )
  .use(initReactI18next)
  .init({
    lng: storedLocale() ?? browserLocale() ?? 'en',
    fallbackLng: 'en',
    supportedLngs: [...SUPPORTED_LOCALES],
    // Keep tags such as zh-Hans intact instead of reducing them to "zh".
    load: 'currentOnly',
    interpolation: { escapeValue: false },
    returnNull: false,
  })

// Keep <html lang> in sync so the browser picks the right CJK glyphs.
i18n.on('languageChanged', (lng) => {
  document.documentElement.lang = lng
})
document.documentElement.lang = i18n.language || 'en'

export default i18n
