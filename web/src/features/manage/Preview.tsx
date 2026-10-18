import { useTranslation } from 'react-i18next'
import type { Schemas } from '@/api/client'
import { Row, RowList } from '@/components/blocks'
import { formatCivilDate, formatTime } from '@/lib/datetime'

type Preview = Schemas['Preview']
/** Open slots per day for the next two weeks under a proposed setting. */
export function PreviewView({ preview, tz }: { preview: Preview; tz: string }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-3">
      <h3>{t('manage.preview.title')}</h3>
      <RowList>
        {preview.days.map((d) => (
          <Row key={d.date} label={formatCivilDate(d.date)}>
            <span className="text-muted-foreground">
              {d.starts.length === 0
                ? t('manage.preview.none')
                : `${t('manage.preview.count', { count: d.starts.length })}: ${d.starts.slice(0, 8).map((s) => formatTime(s, tz)).join(', ')}${d.starts.length > 8 ? '…' : ''}`}
              {d.collapsed && ` ${t('manage.preview.collapsed')}`}
            </span>
          </Row>
        ))}
      </RowList>
    </div>
  )
}
