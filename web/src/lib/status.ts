// Badge colours for every status shown in the back office, in one place so
// the same status looks the same on every page. Normal states are muted;
// colour is kept for states that need attention or report an outcome.
//
// The other visual rules that go with these:
// - Selected (an option picked from a set: a time, a language): Button
//   `selected` — thick primary border, light primary wash, aria-pressed.
// - Emphasised (the next appointment, the earliest time): a thick primary
//   border plus a Badge naming why (size md on customer pages).
// - Dangerous actions: in a row or card, a destructive-outline button whose
//   confirmation is solid red (ConfirmButton does this by default); solid
//   destructive only for a card's main action, such as pausing a calendar.
//   On customer pages cancelling stays outlined (with a red confirmation)
//   so the page does not look alarming.

type StatusBadge = 'success' | 'warning' | 'danger' | 'muted'

export const customerStatusBadge: Record<'pending' | 'approved' | 'rejected' | 'disabled', StatusBadge> = {
  pending: 'warning',
  approved: 'success',
  rejected: 'muted',
  disabled: 'danger',
}

export const appointmentStatusBadge: Record<'booked' | 'completed' | 'cancelled' | 'no_show', StatusBadge> = {
  booked: 'muted',
  completed: 'success',
  cancelled: 'muted',
  no_show: 'warning',
}

export const accountStatusBadge: Record<'active' | 'disabled', StatusBadge> = {
  active: 'muted',
  disabled: 'danger',
}

export const instanceStatusBadge: Record<'active' | 'paused', StatusBadge> = {
  active: 'success',
  paused: 'warning',
}
