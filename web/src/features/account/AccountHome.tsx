import { Link, Navigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/common'
import { Page } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { homePath, useAccount } from './AccountLayout'
import { instanceStatusBadge } from '@/lib/status'

export function AccountHome() {
  const { t } = useTranslation()
  const { me } = useAccount()
  if (me.super_admin) return <Navigate to={homePath(me)} replace />
  return (
    <Page title={t('account.home.title')}>
      {me.instances.length === 0 ? (
        <EmptyState>{t('account.home.none')}</EmptyState>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {me.instances.map((i) => (
            <Link key={i.id} to={`/manage/${i.slug}`} className="block rounded-xl focus-visible:outline-2">
              <Card className="hover:border-primary h-full transition-colors">
                <CardHeader>
                  <CardTitle>{i.name}</CardTitle>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  <Badge variant="secondary">{t(`role.${i.role}`)}</Badge>
                  <Badge variant={instanceStatusBadge[i.status]}>{t(`status.instance.${i.status}`)}</Badge>
                  <span className="text-muted-foreground">{i.timezone}</span>
                </CardContent>
              </Card>
            </Link>
          ))}
        </div>
      )}
    </Page>
  )
}
