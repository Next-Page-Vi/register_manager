import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { $api } from '@/api/client'
import { cursorOptions } from '@/api/queries'
import { LoadMore, Row, RowList } from '@/components/blocks'
import { QueryContent, EmptyState } from '@/components/common'
import { Page } from '@/components/layout'
import { formatDateTime } from '@/lib/datetime'
import { useInstance } from './ManageLayout'

export function actionLabel(t: TFunction, action: string) {
  return t(`audit.actions.${action}`, { defaultValue: action })
}

export function Audit() {
  const { t } = useTranslation()
  const { tz, path } = useInstance()
  const query = $api.useInfiniteQuery(
    'get',
    '/api/v1/instances/{instance_id}/audit',
    { params: { path } },
    cursorOptions,
  )
  const entries = query.data?.pages.flatMap((page) => page.items) ?? []
  return (
    <Page title={t('manage.nav.audit')}>
      <QueryContent query={query}>
        {() =>
          entries.length === 0 ? (
            <EmptyState>{t('audit.empty')}</EmptyState>
          ) : (
            <RowList>
              {entries.map((entry) => (
                <Row key={entry.id} label={formatDateTime(new Date(entry.at), tz)}>
                  {actionLabel(t, entry.action)}
                  <span className="text-muted-foreground">
                    {' · '}
                    {entry.actor_kind === 'cli' ? t('audit.cli') : entry.actor_name}
                  </span>
                </Row>
              ))}
            </RowList>
          )
        }
      </QueryContent>
      <LoadMore query={query} />
    </Page>
  )
}
