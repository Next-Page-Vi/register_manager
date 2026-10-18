import { useState } from 'react'
import { Check, Languages } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Notice } from '@/components/common'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { LOCALE_NAMES, SUPPORTED_LOCALES, currentLocale, storeLocale, type Locale } from '@/i18n'
import { deviceMatchesZone, formatTime, formatZoneName } from '@/lib/datetime'
import { cn } from '@/lib/utils'

/**
 * A language button that opens a dialog for choosing the interface
 * language. With timeZone (a calendar's zone) the dialog also says which
 * time zone the calendar's times are shown in; that zone belongs to the
 * calendar and cannot be changed here.
 *
 * The choice is remembered in this browser; onSave lets a signed-in user's
 * choice be stored in their profile as well. The trigger is always an icon;
 * its accessible name and tooltip include the current language.
 */
export function LanguageSwitcher({ onSave, timeZone, className }: {
  onSave?: (l: Locale) => void
  timeZone?: string
  className?: string
}) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState(false)
  const value = currentLocale()
  const choose = (l: Locale) => {
    setOpen(false)
    if (l === value) return
    storeLocale(l)
    void i18n.changeLanguage(l)
    onSave?.(l)
  }
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {/* Takes the header's text colour, so it reads on any page colour. */}
        <button
          type="button"
          aria-label={t('localeDialog.trigger', { name: LOCALE_NAMES[value] })}
          title={t('localeDialog.trigger', { name: LOCALE_NAMES[value] })}
          className={cn(
            'inline-flex size-11 shrink-0 items-center justify-center rounded-md hover:bg-current/10',
            className,
          )}
        >
          <Languages aria-hidden className="size-5 shrink-0" />
        </button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{timeZone ? t('localeDialog.title') : t('common.language')}</DialogTitle>
        </DialogHeader>
        <section aria-labelledby={timeZone ? 'locale-heading' : undefined} className="space-y-2">
          {timeZone && <h3 id="locale-heading">{t('common.language')}</h3>}
          <div className="grid gap-2">
            {SUPPORTED_LOCALES.map((l) => (
              <Button
                key={l}
                variant="outline"
                lang={l}
                selected={l === value}
                className="justify-between"
                onClick={() => choose(l)}
              >
                {LOCALE_NAMES[l]}
                <Check aria-hidden className={cn('text-primary size-5', l !== value && 'invisible')} />
              </Button>
            ))}
          </div>
        </section>
        {timeZone && (
          <section aria-labelledby="zone-heading" className="space-y-2">
            <h3 id="zone-heading">{t('localeDialog.timezone')}</h3>
            <p>{t('localeDialog.zoneBody', { zone: formatZoneName(timeZone) })}</p>
            <p className="text-muted-foreground">
              {t('localeDialog.zoneNow', { time: formatTime(new Date(), timeZone) })} · {timeZone}
            </p>
            {!deviceMatchesZone(timeZone) && (
              <Notice kind="warning">
                <p>{t('localeDialog.deviceDiffers')}</p>
              </Notice>
            )}
          </section>
        )}
      </DialogContent>
    </Dialog>
  )
}
