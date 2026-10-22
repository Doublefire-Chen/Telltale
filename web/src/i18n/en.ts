// English UI strings. This is the reference locale: every other locale must
// provide exactly these keys (enforced by the Translation type).
export const en = {
  app: {
    name: 'Telltale',
    language: 'Language',
    loading: 'Loading…',
    error: 'Something went wrong. Try reloading the page.',
  },
  setup: {
    title: 'Set an admin password',
    body: 'Telltale has no admin password yet. On the Dashboard machine, run:',
    after: 'Then reload this page.',
  },
  login: {
    title: 'Log in',
    username: 'Username',
    password: 'Password',
    submit: 'Log in',
    invalid: 'Wrong username or password.',
    rateLimited: 'Too many failed attempts. Try again later.',
  },
  status: {
    title: 'Hosts',
    empty: 'No Hosts yet.',
    logout: 'Log out',
  },
} as const

type DeepStrings<T> = { readonly [K in keyof T]: T[K] extends string ? string : DeepStrings<T[K]> }

export type Translation = DeepStrings<typeof en>
