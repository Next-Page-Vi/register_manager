import { useEffect, useState } from 'react'
import { Copy, Download } from 'lucide-react'
import QRCode from 'qrcode'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/**
 * Shows a URL as a QR code (generated locally, no third-party service) with
 * copy and PNG download.
 */
export function QrLink({ url, fileName, label }: { url: string; fileName: string; label: string }) {
  const { t } = useTranslation()
  const [dataUrl, setDataUrl] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let alive = true
    QRCode.toDataURL(url, { width: 640, margin: 2, errorCorrectionLevel: 'M' }).then((d) => {
      if (alive) setDataUrl(d)
    })
    return () => {
      alive = false
    }
  }, [url])

  const copy = async () => {
    try {
      // navigator.clipboard exists only in secure contexts (HTTPS, localhost).
      // eslint-disable-next-line no-restricted-properties -- feature-detected, with fallback below
      const clipboard = navigator.clipboard as Clipboard | undefined
      if (clipboard) {
        await clipboard.writeText(url)
      } else {
        const ta = document.createElement('textarea')
        ta.value = url
        ta.style.position = 'fixed'
        ta.style.opacity = '0'
        document.body.appendChild(ta)
        ta.select()
        const ok = document.execCommand('copy')
        ta.remove()
        if (!ok) throw new Error('copy failed')
      }
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="space-y-3">
      {dataUrl && <img src={dataUrl} alt={label} width={640} height={640} className="mx-auto aspect-square h-auto w-56 max-w-full rounded-md border bg-white p-2" />}
      <Input readOnly value={url} aria-label={label} onFocus={(e) => e.currentTarget.select()} />
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" onClick={copy}>
          <Copy aria-hidden />
          {copied ? t('common.copied') : t('common.copy')}
        </Button>
        {dataUrl && (
          <Button asChild variant="outline">
            <a href={dataUrl} download={fileName}>
              <Download aria-hidden />
              {t('common.downloadQr')}
            </a>
          </Button>
        )}
      </div>
    </div>
  )
}
