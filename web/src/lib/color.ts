// Colour helpers for an instance's page colour ("#rrggbb").

const HEX = /^#[0-9a-f]{6}$/i

export function isHexColor(s: string | null | undefined): s is string {
  return !!s && HEX.test(s)
}

const LIGHT_TEXT = '#ffffff'
// Pure black: with any softer black some colours fall below 4.5:1.
const DARK_TEXT = '#000000'

/** WCAG relative luminance of "#rrggbb". */
function luminance(hex: string) {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

export function contrast(a: string, b: string) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x)
  return (hi + 0.05) / (lo + 0.05)
}

/**
 * Text colour for a background: white or black, whichever contrasts more.
 * One of the two always reaches at least 4.58:1 (checked over every
 * colour), so any page colour keeps the header readable.
 */
export function readableOn(bg: string) {
  return contrast(bg, LIGHT_TEXT) >= contrast(bg, DARK_TEXT) ? LIGHT_TEXT : DARK_TEXT
}
