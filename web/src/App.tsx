import { lazy, Suspense } from 'react'
import { createBrowserRouter, createRoutesFromElements, Navigate, Route, RouterProvider } from 'react-router'
import { BootSpinner } from '@/components/common'
import { BookingHome } from '@/features/customer/BookingHome'
import { ConfirmPage } from '@/features/customer/ConfirmPage'
import { CustomerLayout } from '@/features/customer/CustomerLayout'
import { DonePage } from '@/features/customer/DonePage'
import { LinkInvalidPage, NotFoundPage, RegisterDonePage, RegisterPage } from '@/features/customer/RegisterPage'
import { TimesPage } from '@/features/customer/TimesPage'

// The back office is loaded on demand so customers' phones download less.
const AccountHome = lazy(() => import('@/features/account/AccountHome').then((m) => ({ default: m.AccountHome })))
const AccountLayout = lazy(() => import('@/features/account/AccountLayout').then((m) => ({ default: m.AccountLayout })))
const RequireSuperAdmin = lazy(() => import('@/features/account/AccountLayout').then((m) => ({ default: m.RequireSuperAdmin })))
const ProfilePage = lazy(() => import('@/features/account/ProfilePage').then((m) => ({ default: m.ProfilePage })))
const LoginPage = lazy(() => import('@/features/account/LoginPage').then((m) => ({ default: m.LoginPage })))
const Appointments = lazy(() => import('@/features/manage/Appointments').then((m) => ({ default: m.Appointments })))
const Audit = lazy(() => import('@/features/manage/Audit').then((m) => ({ default: m.Audit })))
const Dashboard = lazy(() => import('@/features/manage/Dashboard').then((m) => ({ default: m.Dashboard })))
const StatisticsPage = lazy(() => import('@/features/manage/Statistics').then((m) => ({ default: m.StatisticsPage })))
const ManageLayout = lazy(() => import('@/features/manage/ManageLayout').then((m) => ({ default: m.ManageLayout })))
const PolicyPage = lazy(() => import('@/features/manage/Policy').then((m) => ({ default: m.PolicyPage })))
const Schedule = lazy(() => import('@/features/manage/Schedule').then((m) => ({ default: m.Schedule })))
const Settings = lazy(() => import('@/features/manage/Settings').then((m) => ({ default: m.Settings })))
const AccountsPage = lazy(() => import('@/features/platform/AccountsPage').then((m) => ({ default: m.AccountsPage })))
const SettingsPage = lazy(() => import('@/features/platform/SettingsPage').then((m) => ({ default: m.SettingsPage })))
const InstancesPage = lazy(() => import('@/features/platform/InstancesPage').then((m) => ({ default: m.InstancesPage })))
const Customers = lazy(() => import('@/features/manage/Customers').then((m) => ({ default: m.Customers })))
const CustomerDetail = lazy(() => import('@/features/manage/Customers').then((m) => ({ default: m.CustomerDetail })))

const router = createBrowserRouter(createRoutesFromElements(
  <>
    <Route path="/" element={<Navigate to="/account" replace />} />
    <Route path="/account/login" element={<LoginPage />} />
    <Route element={<AccountLayout />}>
      <Route path="/account" element={<AccountHome />} />
      <Route path="/account/profile" element={<ProfilePage />} />
      <Route element={<RequireSuperAdmin />}>
        <Route path="/platform/accounts" element={<AccountsPage />} />
        <Route path="/platform/instances" element={<InstancesPage />} />
        <Route path="/platform/settings" element={<SettingsPage />} />
      </Route>
      <Route path="/manage/:slug" element={<ManageLayout />}>
        <Route index element={<Dashboard />} />
        <Route path="appointments" element={<Appointments />} />
        <Route path="statistics" element={<StatisticsPage />} />
        <Route path="customers" element={<Customers />} />
        <Route path="customers/:customerId" element={<CustomerDetail />} />
        <Route path="schedule" element={<Schedule />} />
        <Route path="policy" element={<PolicyPage />} />
        <Route path="settings" element={<Settings />} />
        <Route path="audit" element={<Audit />} />
      </Route>
    </Route>
    <Route path="/i/:slug/register" element={<RegisterPage />} />
    <Route path="/i/:slug/register/done" element={<RegisterDonePage />} />
    <Route path="/i/:slug/link-invalid" element={<LinkInvalidPage />} />
    <Route path="/i/:slug/booking" element={<CustomerLayout />}>
      <Route index element={<BookingHome />} />
      <Route path="times" element={<TimesPage />} />
      <Route path="confirm" element={<ConfirmPage />} />
      <Route path="done/:appointmentId" element={<DonePage />} />
    </Route>
    <Route path="*" element={<NotFoundPage />} />
  </>,
))

export function App() {
  return <Suspense fallback={<BootSpinner />}><RouterProvider router={router} /></Suspense>
}
