/** Runs in the browser: a control must fit its field, not just the viewport. */
export function formControlLayoutIssues(root: HTMLElement): string[] {
  const issues: string[] = []
  for (const el of Array.from(root.querySelectorAll<HTMLElement>('[data-slot=input], textarea, select'))) {
    const r = el.getBoundingClientRect()
    const s = getComputedStyle(el)
    if (!r.width || !r.height || s.visibility === 'hidden' || el.closest('.sr-only')) continue
    const name = el.id || el.getAttribute('aria-label') || el.tagName.toLowerCase()
    const parent = el.parentElement!
    const p = parent.getBoundingClientRect()
    const ps = getComputedStyle(parent)
    const left = p.left + parseFloat(ps.borderLeftWidth) + parseFloat(ps.paddingLeft)
    const right = p.right - parseFloat(ps.borderRightWidth) - parseFloat(ps.paddingRight)
    if (r.left < left - 1 || r.right > right + 1) {
      issues.push(`${name}: control leaves its field (${r.left.toFixed(1)}..${r.right.toFixed(1)} outside ${left.toFixed(1)}..${right.toFixed(1)})`)
    }
    if (el instanceof HTMLInputElement && ['date', 'time', 'datetime-local'].includes(el.type)) {
      // Linux WebKit does not run the iOS native theme. Guard its workaround
      // explicitly: that theme can override border-box and min-width even
      // when ordinary geometry checks pass on Linux (WebKit bug 301648).
      if (s.appearance !== 'none' || s.boxSizing !== 'border-box') {
        issues.push(`${name}: native iOS date/time sizing is not disabled`)
      }
      // Ordinary text fields intentionally scroll long editable text. Date
      // and time values, however, should fit without horizontal scrolling.
      if (el.scrollWidth > el.clientWidth + 1) {
        issues.push(`${name}: date/time content overflows (${el.scrollWidth} > ${el.clientWidth})`)
      }
    }
  }
  return issues
}
