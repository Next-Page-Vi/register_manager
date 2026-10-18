/** All IANA time zones supported by the browser. */
export function timeZones(): string[] {
  try {
    return Intl.supportedValuesOf('timeZone')
  } catch {
    return ['UTC']
  }
}
