import type { Translation } from './en'

// Simplified Chinese UI strings.
export const zhCN: Translation = {
  app: {
    name: 'Telltale',
    language: '语言',
    loading: '加载中…',
    error: '出错了，请刷新页面重试。',
  },
  setup: {
    title: '设置管理员密码',
    body: 'Telltale 尚未设置管理员密码。请在 Dashboard 所在机器上运行：',
    after: '然后刷新本页。',
  },
  login: {
    title: '登录',
    username: '用户名',
    password: '密码',
    submit: '登录',
    invalid: '用户名或密码错误。',
    rateLimited: '失败次数过多，请稍后再试。',
  },
  status: {
    title: '主机',
    empty: '暂无主机。',
    logout: '退出登录',
  },
}
