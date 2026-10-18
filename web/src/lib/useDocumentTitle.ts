import { useEffect } from 'react'

/** Joins title parts, most specific first, skipping empty ones. */
export function joinTitle(...parts: (string | null | undefined)[]) {
  return parts.filter(Boolean).join(' · ')
}

/**
 * Sets the browser tab title (also used when a page is shared in a chat
 * app). null leaves it to a nested route that sets its own.
 */
export function useDocumentTitle(title: string | null) {
  useEffect(() => {
    if (title) document.title = title
  }, [title])
}
