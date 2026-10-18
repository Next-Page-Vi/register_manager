import { Component, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

function CrashedPage() {
  const { t } = useTranslation()
  return (
    <main className="mx-auto flex min-h-svh max-w-md flex-col items-center justify-center gap-6 p-4 text-center">
      <p className="text-lg">{t('common.crashed')}</p>
      <Button type="button" onClick={() => window.location.reload()}>
        {t('common.reload')}
      </Button>
    </main>
  )
}

/** Shows a reload prompt instead of a blank page when rendering throws. */
export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: unknown) {
    console.error(error)
  }

  render() {
    return this.state.failed ? <CrashedPage /> : this.props.children
  }
}
