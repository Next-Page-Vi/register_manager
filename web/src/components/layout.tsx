import type { ReactNode } from 'react'
import { ArrowLeft } from 'lucide-react'
import { Link } from 'react-router'
import { Button } from '@/components/ui/button'
import { ActionGroup } from '@/components/ActionGroup'

/*
 * Page structure and spacing. Every page follows these steps so pages line
 * up with each other and with the header; prefer the components here over
 * ad hoc margins.
 *
 *   Between page blocks: --page-gap (20px staff, 24px customer).
 *   Within sections and between detail pairs: gap/space-y-3 (0.75rem).
 *   Within cards: gap/space-y-4 (1rem).
 *   Between form fields: --form-gap (16px staff, 22.5px customer).
 *   Label/control and action groups: gap/space-y-2 (0.5rem).
 *   Root units respect the browser font size; Surface owns customer sizing.
 *   Box padding: list rows p-3; notices, empty states and other boxes p-4;
 *   cards use --card-padding-inline/block, list cards py-4.
 *   Page padding: py-6 in the back office (owned by layouts).
 *   Calendar context: mb-4 above the page (ManageLayout).
 *   The booking result on the done page is the one larger box (p-6 sm:p-8).
 *   Header controls: size/min-h-11; customer headers use py-3, staff py-2.
 *
 * Width: every back office page, forms included, fills the header's
 * 90rem container, so its left and right edges line up with the
 * header's; on wider screens the whole container is centred. Customer pages
 * are one centred 47.25rem column. See DESIGN.md for the full scale.
 */

/**
 * A page heading with optional actions on the same row. The row is as tall
 * as a button either way, so titles sit at the same height on every page.
 */
export function PageTitle({ children, description, actions }: {
  children: ReactNode
  description?: ReactNode
  actions?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
      <div className="min-w-0 flex-[1_1_12rem] space-y-2">
        <h1 className="flex min-h-11 items-center">{children}</h1>
        {description && <p className="max-w-prose text-muted-foreground">{description}</p>}
      </div>
      {actions && <ActionGroup className="self-start">{actions}</ActionGroup>}
    </div>
  )
}

/** A "back" link whose arrow lines up with the text edge below it. */
export function BackLink({ to, children }: { to: string; children: ReactNode }) {
  return (
    <Button asChild variant="ghost" className="-ms-4 px-4">
      <Link to={to}>
        <ArrowLeft aria-hidden />
        {children}
      </Link>
    </Button>
  )
}

/**
 * The body of a page: an optional back link, booking steps, the title
 * with its description and actions, then the
 * content, with spacing supplied by the current surface.
 */
export function Page({ title, description, actions, back, steps, children }: {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  back?: { to: string; label: ReactNode }
  steps?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="space-y-(--page-gap)">
      {back && <BackLink to={back.to}>{back.label}</BackLink>}
      {steps}
      {title && <PageTitle description={description} actions={actions}>{title}</PageTitle>}
      {children}
    </div>
  )
}
