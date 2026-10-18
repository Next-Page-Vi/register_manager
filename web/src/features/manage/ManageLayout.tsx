import { createContext, Suspense, useContext, useMemo } from 'react'
import { ChartNoAxesCombined, CalendarClock, CalendarDays, Clock3, History, Settings2, SlidersHorizontal, Users } from 'lucide-react'
import { NavLink, Outlet, useLocation, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { ErrorNotice, Loading, navItemClass } from '@/components/common'
import { Badge } from '@/components/ui/badge'
import { formatZoneName } from '@/lib/datetime'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { usePhoneMenuSection } from '../account/AccountLayout'
import { instanceStatusBadge } from '@/lib/status'

type Access = Schemas['InstanceAccess']

type InstanceContext = {
  access: Access
  id: string
  /** Path parameters naming the instance, for API calls. */
  path: { instance_id: string }
  tz: string
}

const Ctx = createContext<InstanceContext | null>(null)

/** The instance being managed, its time zone and the caller's role. */
export function useInstance() {
  const v = useContext(Ctx)
  if (!v) throw new Error('useInstance outside ManageLayout')
  return v
}

const SECTIONS = [
  { to: '', key: 'dashboard', group: 'daily', icon: CalendarDays, end: true },
  { to: 'appointments', key: 'appointments', group: 'daily', icon: CalendarClock },
  { to: 'customers', key: 'customers', group: 'daily', icon: Users },
  { to: 'statistics', key: 'statistics', group: 'records', icon: ChartNoAxesCombined },
  { to: 'schedule', key: 'schedule', group: 'configuration', icon: Clock3 },
  { to: 'policy', key: 'policy', group: 'configuration', icon: SlidersHorizontal },
  { to: 'settings', key: 'settings', group: 'configuration', icon: Settings2 },
  { to: 'audit', key: 'audit', group: 'records', icon: History },
] as const

const GROUPS = ['daily', 'configuration', 'records'] as const

function SectionNav({ slug }: { slug: string }) {
  const { t } = useTranslation()
  return (
    <nav aria-label={t('nav.instance')} className="space-y-5">
      {GROUPS.map((group) => (
        <div key={group} className="space-y-2">
          <p className="px-3 text-sm text-muted-foreground">{t(`manage.navGroups.${group}`)}</p>
          <div className="space-y-1">
            {SECTIONS.filter((s) => s.group === group).map((s) => (
              <NavLink
                key={s.key}
                to={`/manage/${slug}${s.to ? `/${s.to}` : ''}`}
                end={'end' in s}
                className={({ isActive }) => navItemClass(isActive, 'vertical')}
              >
                <s.icon aria-hidden className="size-5 shrink-0" />
                {t(`manage.nav.${s.key}`)}
              </NavLink>
            ))}
          </div>
        </div>
      ))}
    </nav>
  )
}

export function ManageLayout() {
  const { slug } = useParams()
  if (!slug) throw new Error('Missing route parameter: slug')
  const { t } = useTranslation()
  const query = $api.useQuery('get', '/api/v1/instances/by-slug/{slug}', { params: { path: { slug } } })
  const { pathname } = useLocation()
  const section = SECTIONS.find((s) => s.to && pathname.startsWith(`/manage/${slug}/${s.to}`)) ?? SECTIONS[0]
  const instName = query.status === 'success' ? query.data.instance.name : null
  useDocumentTitle(joinTitle(t(`manage.nav.${section.key}`), instName, t('app.name')))
  const instTz = query.status === 'success' ? query.data.instance.timezone : null
  // On phones the sections live in the main menu, under the calendar's name;
  // the language dialog names the calendar's time zone.
  const menuSection = useMemo(
    () => (instName && instTz ? { title: instName, nav: <SectionNav slug={slug} />, timeZone: instTz } : null),
    [instName, instTz, slug],
  )
  usePhoneMenuSection(menuSection)
  const access = query.data ?? null
  // One value per loaded instance, so consumers do not re-render with
  // every render of the layout.
  const ctx = useMemo(
    () =>
      access && {
        access,
        id: access.instance.id,
        path: { instance_id: access.instance.id },
        tz: access.instance.timezone,
      },
    [access],
  )
  if (query.status === 'pending') return <Loading />
  if (query.status === 'error' && query.data === undefined) return <ErrorNotice error={query.error} />
  if (!ctx) return null
  const inst = ctx.access.instance
  return (
    <Ctx.Provider value={ctx}>
      <div className="flex gap-6">
        <aside className="hidden w-52 shrink-0 lg:block">
          <div className="sticky top-6">
            <SectionNav slug={slug} />
          </div>
        </aside>
        <div className="min-w-0 flex-1">
          <div className="mb-4 space-y-2 border-b pb-3">
            <p className="font-semibold">{inst.name}</p>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
              <Badge variant={instanceStatusBadge[inst.status]}>{t(`status.instance.${inst.status}`)}</Badge>
              <span className="text-sm text-muted-foreground" title={inst.timezone}>
                {t('manage.timezone', { zone: formatZoneName(inst.timezone) })}
              </span>
            </div>
          </div>
          <Suspense fallback={<Loading />}>
            <Outlet />
          </Suspense>
        </div>
      </div>
    </Ctx.Provider>
  )
}
