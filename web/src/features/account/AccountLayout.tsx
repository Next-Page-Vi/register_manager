import { createContext, Suspense, useContext, useEffect, useState, type ReactNode } from 'react'
import { CalendarCheck, LogOut, Menu } from 'lucide-react'
import { Link, Navigate, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, UNAUTHENTICATED_EVENT, type SessionScope, type Schemas } from '@/api/client'
import { BootSpinner, ErrorNotice, Loading, navItemClass, Notice } from '@/components/common'
import { UnsavedChangesProvider, useLeaveGuard } from '@/components/UnsavedChanges'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { applyLocale } from '@/i18n'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { Surface } from '@/components/Surface'

type AccountMe = Schemas['AccountMe']

const Ctx = createContext<{ me: AccountMe } | null>(null)

export function useAccount() {
  const v = useContext(Ctx)
  if (!v) throw new Error('useAccount outside AccountLayout')
  return v
}

/** Platform pages: only the super admin may open them. */
export function RequireSuperAdmin() {
  const { t } = useTranslation()
  const { me } = useAccount()
  if (!me.super_admin)
    return (
      <Notice kind="error">
        <p>{t('errors.forbidden')}</p>
      </Notice>
    )
  return <Outlet />
}

/** Where a signed-in account starts: its calendars, or all calendars for the super admin. */
export function homePath(me: { super_admin: boolean }) {
  return me.super_admin ? '/platform/instances' : '/account'
}

/**
 * The open calendar, as its layout tells the frame: its name and section
 * links for the top of the phone menu, and its time zone for the language
 * dialog.
 */
type MenuSection = { title: ReactNode; nav: ReactNode; timeZone: string }

const MenuCtx = createContext<(s: MenuSection | null) => void>(() => {})

/**
 * Puts a page's own navigation (a calendar's sections) into the phone menu
 * while the page is shown. Pass a stable value (memoised) to avoid loops.
 */
export function usePhoneMenuSection(section: MenuSection | null) {
  const set = useContext(MenuCtx)
  useEffect(() => {
    set(section)
    return () => set(null)
  }, [set, section])
}

/** Signed-in back office frame; redirects to login without a session. */
export function AccountLayout() {
  return <UnsavedChangesProvider><AccountFrame /></UnsavedChangesProvider>
}

function AccountFrame() {
  const { requestLeave, forceLeave } = useLeaveGuard()
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const query = $api.useQuery('get', '/api/v1/account/me')
  const logout = $api.useMutation('post', '/api/v1/account/logout', {
    onSuccess: () => navigate('/account/login', { replace: true }),
  })
  const locale = $api.useMutation('put', '/api/v1/account/locale')
  const saveLocale = (value: string) => locale.mutate({ body: { locale: value } })
  const [menuOpen, setMenuOpen] = useState(false)
  const [menuSection, setMenuSection] = useState<MenuSection | null>(null)
  const me = query.data?.account ?? null

  // Instance pages set their own, more specific title.
  const page = (
    {
      '/account': t('nav.myInstances'),
      '/platform/instances': t('nav.instances'),
      '/platform/accounts': t('nav.accounts'),
      '/account/profile': t('nav.profile'),
      '/platform/settings': t('nav.settings'),
    } as Record<string, string | undefined>
  )[location.pathname]
  useDocumentTitle(location.pathname.startsWith('/manage/') ? null : joinTitle(page, t('app.name')))

  useEffect(() => {
    if (me) applyLocale({ profile: me.locale })
  }, [me])

  useEffect(() => {
    const onUnauth = (event: Event) => {
      if ((event as CustomEvent<SessionScope>).detail.kind === 'account')
        forceLeave(() => navigate(`/account/login?next=${encodeURIComponent(location.pathname)}`, { replace: true }))
    }
    window.addEventListener(UNAUTHENTICATED_EVENT, onUnauth)
    return () => window.removeEventListener(UNAUTHENTICATED_EVENT, onUnauth)
  }, [navigate, location.pathname, forceLeave])

  if (query.status === 'pending') return <BootSpinner />
  if (query.status === 'error' && query.data === undefined)
    return (
      <div className="mx-auto max-w-xl p-4">
        <ErrorNotice error={query.error} />
      </div>
    )
  if (!me) return <Navigate to={`/account/login?next=${encodeURIComponent(location.pathname)}`} replace />
  // The super admin must replace the generated initial password first.
  if (me.must_change_password && location.pathname !== '/account/profile')
    return <Navigate to="/account/profile" replace />

  // The super admin opens every calendar from "All calendars", so has no
  // "My calendars". The calendar list stays marked while a calendar is open.
  const home = homePath(me)
  const mainNav = (vertical: boolean) => {
    const orientation = vertical ? 'vertical' : 'horizontal'
    const cls = ({ isActive }: { isActive: boolean }) => navItemClass(isActive, orientation)
    const listCls = ({ isActive }: { isActive: boolean }) =>
      cls({ isActive: isActive || location.pathname.startsWith('/manage/') })
    return (
      <nav aria-label={t('nav.main')} className={vertical ? 'flex flex-col gap-1' : 'flex flex-wrap gap-1'}>
        {!me.super_admin && (
          <NavLink to="/account" end className={listCls}>
            {t('nav.myInstances')}
          </NavLink>
        )}
        {me.super_admin && !me.must_change_password && (
          <>
            <NavLink to="/platform/instances" className={listCls}>
              {t('nav.instances')}
            </NavLink>
            <NavLink to="/platform/accounts" className={cls}>
              {t('nav.accounts')}
            </NavLink>
            <NavLink to="/platform/settings" className={cls}>
              {t('nav.settings')}
            </NavLink>
          </>
        )}
        <NavLink to="/account/profile" className={cls}>
          {t('nav.profile')}
        </NavLink>
      </nav>
    )
  }

  return (
    <Ctx.Provider value={{ me }}>
      <MenuCtx.Provider value={setMenuSection}>
        <Surface kind="staff" className="min-h-dvh bg-canvas">
          <a href="#main-content" className="sr-only focus:not-sr-only focus:fixed focus:start-4 focus:top-4 focus:z-50 focus:rounded-md focus:bg-background focus:p-3 focus:text-primary">{t('nav.skipToContent')}</a>
          <header className="border-b bg-background">
            <div className="mx-auto flex min-h-16 max-w-[90rem] flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2 sm:px-6">
              {/* Phones and tablets: one row with the menu, the name and sign out. */}
              <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
                <SheetTrigger asChild>
                  <Button variant="outline" size="icon" className="size-11 lg:hidden" aria-label={t('nav.menu')}>
                    <Menu aria-hidden />
                  </Button>
                </SheetTrigger>
                <SheetContent
                  side="left"
                  className="w-[min(20rem,88vw)] gap-0 overflow-y-auto p-4"
                  // Following any link closes the menu, including the current page's.
                  onClick={(e) => {
                    if ((e.target as Element).closest('a')) setMenuOpen(false)
                  }}
                >
                  <SheetHeader className="ps-0">
                    <SheetTitle>{t('nav.menu')}</SheetTitle>
                  </SheetHeader>
                  {menuSection && (
                    <div className="mb-4 space-y-2 border-b pb-4">
                      <p className="text-muted-foreground px-3 font-semibold">{menuSection.title}</p>
                      {menuSection.nav}
                    </div>
                  )}
                  {mainNav(true)}
                  <p className="text-muted-foreground mt-4 border-t px-3 pt-4">{me.name}</p>
                </SheetContent>
              </Sheet>
              <Link to={home} className="text-primary inline-flex min-h-11 items-center gap-2 text-lg font-bold">
                <CalendarCheck aria-hidden />
                {t('app.name')}
              </Link>
              <div className="hidden lg:block">{mainNav(false)}</div>
              <div className="ms-auto flex min-w-0 items-center gap-3">
                <span className="text-muted-foreground hidden max-w-36 truncate xl:inline" title={me.name}>{me.name}</span>
                <div className="flex items-center gap-2">
                  <LanguageSwitcher onSave={saveLocale} timeZone={menuSection?.timeZone} />
                  {/* On phones only the icon shows; the label names the button. */}
                  <Button
                    variant="ghost"
                    className="min-h-11"
                    onClick={() => requestLeave(() => logout.mutateAsync({}))}
                    pending={logout.isPending}
                    aria-label={t('nav.logout')}
                  >
                    <LogOut aria-hidden />
                    <span className="hidden sm:inline">{t('nav.logout')}</span>
                  </Button>
                </div>
              </div>
            </div>
          </header>
          {/* Same width as the header, so every page starts at the logo's edge. */}
          <main id="main-content" tabIndex={-1} className="mx-auto max-w-[90rem] px-4 py-6 outline-none sm:px-6">
            <ErrorNotice error={logout.error ?? locale.error} />
            <Suspense fallback={<Loading />}>
              <Outlet />
            </Suspense>
          </main>
        </Surface>
      </MenuCtx.Provider>
    </Ctx.Provider>
  )
}
