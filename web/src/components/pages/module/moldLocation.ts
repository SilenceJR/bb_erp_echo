export const PALLET_LOCATION_CODE = '卡板'

export type MoldLocationOption = {
  id: number
  code: string
  status?: 'active' | 'disabled' | string
}

export type ShelfLocationParts = {
  zone: string
  row: number
  column: number
}

// Keep the legacy A1-1 wire format at the UI boundary. Custom historical
// codes are intentionally left untouched when they do not match this shape.
export function parseShelfLocationCode(code: string): ShelfLocationParts | null {
  const match = code.trim().match(/^([A-Za-z]{1,8})(\d+)-(\d+)$/)
  if (!match) return null
  return {zone: match[1].toUpperCase(), row: Number(match[2]), column: Number(match[3])}
}

export function formatShelfLocationCode(zone: string, row: number | string, column: number | string): string {
  return `${zone.trim().toUpperCase()}${Number(row)}-${Number(column)}`
}

export function isPalletLocation(location?: MoldLocationOption | null): boolean {
  return location?.code === PALLET_LOCATION_CODE
}

export function shelfLocations(locations: MoldLocationOption[]): MoldLocationOption[] {
  return locations.filter((location) => Boolean(parseShelfLocationCode(location.code)))
}

export function locationByCode(locations: MoldLocationOption[], code: string): MoldLocationOption | undefined {
  return locations.find((location) => location.code === code)
}

export function generatedShelfCodes(zone: string, rows: number, columns: number): string[] {
  const normalizedZone = zone.trim().toUpperCase()
  const normalizedRows = Math.max(0, Math.floor(Number(rows) || 0))
  const normalizedColumns = Math.max(0, Math.floor(Number(columns) || 0))
  const codes: string[] = []
  for (let row = 1; row <= normalizedRows; row += 1) {
    for (let column = 1; column <= normalizedColumns; column += 1) {
      codes.push(formatShelfLocationCode(normalizedZone, row, column))
    }
  }
  return codes
}

export function missingShelfCodes(locations: MoldLocationOption[], zone: string, rows: number, columns: number): string[] {
  const existing = new Set(shelfLocations(locations).map((location) => location.code.toUpperCase()))
  return generatedShelfCodes(zone, rows, columns).filter((code) => !existing.has(code))
}
