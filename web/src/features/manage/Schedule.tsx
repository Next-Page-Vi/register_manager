import { useTranslation } from 'react-i18next'
import { Page } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { CycleEditor } from './schedule/CycleEditor'
import { BlocksEditor, OverridesEditor } from './schedule/DatedEntries'

const SECTIONS = ['cycle', 'overrides', 'blocks'] as const

/** The repeating schedule, special dates and temporary blocks. */
export function Schedule() {
  const { t } = useTranslation()
  return (
    <Page title={t('manage.schedule.title')}>
      <nav aria-label={t('manage.schedule.sections')} className="flex flex-wrap gap-2">
        {SECTIONS.map((section) => (
          <Button key={section} asChild variant="outline" size="sm">
            <a href={`#schedule-${section}`}>{t(`manage.schedule.${section}Title`)}</a>
          </Button>
        ))}
      </nav>
      <section id="schedule-cycle" aria-label={t('manage.schedule.cycleTitle')} className="scroll-mt-6" tabIndex={-1}>
        <CycleEditor />
      </section>
      <section id="schedule-overrides" aria-label={t('manage.schedule.overridesTitle')} className="scroll-mt-6" tabIndex={-1}>
        <OverridesEditor />
      </section>
      <section id="schedule-blocks" aria-label={t('manage.schedule.blocksTitle')} className="scroll-mt-6" tabIndex={-1}>
        <BlocksEditor />
      </section>
    </Page>
  )
}
