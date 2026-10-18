/** A random UUID used as a client-generated ID for idempotent submits. */
export function newId(): string {
  // crypto.randomUUID exists only in secure contexts (HTTPS, localhost);
  // getRandomValues also works over plain HTTP and on older browsers.
  const b = crypto.getRandomValues(new Uint8Array(16))
  b[6] = (b[6] & 0x0f) | 0x40 // version 4
  b[8] = (b[8] & 0x3f) | 0x80 // RFC 9562 variant
  const h = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`
}

/**
 * Returns an ID that stays the same for the same purpose within this tab,
 * so a refresh or going back and submitting again reuses it and the server
 * returns the original result instead of creating a duplicate.
 */
export function stickyId(purpose: string): string {
  const key = `rm_id_${purpose}`
  try {
    const existing = sessionStorage.getItem(key)
    if (existing) return existing
    const id = newId()
    sessionStorage.setItem(key, id)
    return id
  } catch {
    return newId()
  }
}

export function clearStickyId(purpose: string) {
  try {
    sessionStorage.removeItem(`rm_id_${purpose}`)
  } catch {
    // ignore
  }
}
