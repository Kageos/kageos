import type { SupportedLocale } from '@/architecture/shared/i18n'

export type KageosDocSlug =
  | 'docs'
  | 'architecture'
  | 'connectors'
  | 'login'
  | 'runtime'
  | 'data-backup'
  | 'permissions'
  | 'automation'
  | 'hub'
  | 'operations'
  | 'api'

const kageosDocsBaseURL = 'https://kageos.ai'
const kageosWebsiteURL = 'https://kageos.com'
const kageosHubBaseURL = 'https://hub.kageos.com'

export function getKageosDocsURL(slug: KageosDocSlug = 'docs', locale?: SupportedLocale | string): string {
  const docsPrefix = locale?.toLowerCase().startsWith('zh') ? '/zh/docs' : '/docs'
  const suffix = slug === 'docs' ? '' : `/${slug}`
  return `${kageosDocsBaseURL}${docsPrefix}${suffix}`
}

export function getKageosHubURL(): string {
  return kageosHubBaseURL
}

export function getKageosWebsiteURL(): string {
  return kageosWebsiteURL
}

export function openExternalURL(url: string): void {
  window.open(url, '_blank', 'noopener,noreferrer')
}
