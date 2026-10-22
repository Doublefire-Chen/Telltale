import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'
import { en } from './en'
import { zhCN } from './zh-CN'

export const languages = [
  { code: 'en', label: 'English' },
  { code: 'zh-CN', label: '简体中文' },
] as const

export type Language = (typeof languages)[number]['code']

export const languageStorageKey = 'telltale.language'

declare module 'i18next' {
  interface CustomTypeOptions {
    resources: { translation: typeof en }
  }
}

// Any Chinese browser locale gets Simplified Chinese; everything else gets English.
export function toSupported(lng: string): Language {
  return lng.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en'
}

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: { en: { translation: en }, 'zh-CN': { translation: zhCN } },
    supportedLngs: languages.map((l) => l.code),
    fallbackLng: 'en',
    interpolation: { escapeValue: false },
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: languageStorageKey,
      caches: ['localStorage'],
      convertDetectedLanguage: toSupported,
    },
  })

function syncDocumentLanguage(lng: string) {
  document.documentElement.lang = lng
}
syncDocumentLanguage(i18n.resolvedLanguage ?? 'en')
i18n.on('languageChanged', syncDocumentLanguage)

export default i18n
