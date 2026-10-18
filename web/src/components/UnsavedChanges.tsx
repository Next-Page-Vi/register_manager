import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useBeforeUnload, useBlocker } from 'react-router'
import { useTranslation } from 'react-i18next'
import { AlertDialog, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'

type Status = 'clean' | 'dirty' | 'saving'
type LeaveAction = () => void | Promise<unknown>
type Guard = {
  setStatus: (status: Status) => void
  requestLeave: (run: LeaveAction) => void
  forceLeave: (run: LeaveAction) => void
}
const Context = createContext<Guard | null>(null)

/** One blocker for the back office; only the mounted long form registers a draft. */
export function UnsavedChangesProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [status, setStatus] = useState<Status>('clean')
  const [action, setAction] = useState<{ run: LeaveAction } | null>(null)
  const bypass = useRef(false)
  const blocker = useBlocker(({ currentLocation, nextLocation }) =>
    !bypass.current && status !== 'clean' && currentLocation.pathname !== nextLocation.pathname,
  )
  useBeforeUnload(useCallback((event) => {
    if (!bypass.current && status !== 'clean') {
      event.preventDefault()
      event.returnValue = true
    }
  }, [status]))
  const forceLeave = useCallback((run: LeaveAction) => {
    bypass.current = true
    // Mutation errors remain visible in the frame. A failed logout keeps the draft.
    void Promise.resolve().then(run).catch(() => {}).finally(() => { bypass.current = false })
  }, [])
  const requestLeave = useCallback((run: LeaveAction) => {
    if (status === 'clean') forceLeave(run)
    else setAction({ run })
  }, [status, forceLeave])
  const guard = useMemo(() => ({ setStatus, requestLeave, forceLeave }), [requestLeave, forceLeave])
  const stay = () => {
    setAction(null)
    if (blocker.state === 'blocked') blocker.reset()
  }
  const leave = () => {
    if (status === 'saving') return
    if (action) {
      const run = action.run
      setAction(null)
      forceLeave(run)
    } else if (blocker.state === 'blocked') blocker.proceed()
  }
  useEffect(() => {
    if (status !== 'clean') return
    if (blocker.state === 'blocked') blocker.proceed()
    if (action) {
      setAction(null)
      forceLeave(action.run)
    }
  }, [status, blocker, action, forceLeave])
  return (
    <Context.Provider value={guard}>
      {children}
      <AlertDialog open={blocker.state === 'blocked' || action !== null} onOpenChange={(open) => { if (!open) stay() }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('common.unsaved.title')}</AlertDialogTitle>
            <AlertDialogDescription>{t(status === 'saving' ? 'common.unsaved.saving' : 'common.unsaved.description')}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={stay}>{t('common.unsaved.stay')}</AlertDialogCancel>
            <Button variant="destructive" disabled={status === 'saving'} onClick={leave}>{t('common.unsaved.discard')}</Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Context.Provider>
  )
}

export function useLeaveGuard() {
  const guard = useContext(Context)
  if (!guard) throw new Error('useLeaveGuard outside UnsavedChangesProvider')
  return guard
}

export function useUnsavedChanges(dirty: boolean, saving: boolean) {
  const { setStatus } = useLeaveGuard()
  useLayoutEffect(() => {
    setStatus(saving ? 'saving' : dirty ? 'dirty' : 'clean')
    return () => setStatus('clean')
  }, [dirty, saving, setStatus])
}
