export const themeStorageKey = 'bb_erp_theme_mode'
export const accentStorageKey = 'bb_erp_theme_accent'
export const sidebarStorageKey = 'bb_erp_sidebar_mode'

export type ThemeMode = 'light' | 'dark'
export type AccentTheme = 'bobbang' | 'teal' | 'violet'
export type SidebarMode = 'full' | 'icon' | 'hidden'

const themeModes: ThemeMode[] = ['light', 'dark']
const accentThemes: AccentTheme[] = ['bobbang', 'teal', 'violet']
const sidebarModes: SidebarMode[] = ['full', 'icon', 'hidden']

export function normalizeThemeMode(value: string | null): ThemeMode {
  return themeModes.includes(value as ThemeMode) ? value as ThemeMode : 'light'
}

export function normalizeAccentTheme(value: string | null): AccentTheme {
  return accentThemes.includes(value as AccentTheme) ? value as AccentTheme : 'bobbang'
}

export function normalizeSidebarMode(value: string | null): SidebarMode {
  return sidebarModes.includes(value as SidebarMode) ? value as SidebarMode : 'full'
}

export function applyAppearance(
  root: Pick<HTMLElement, 'dataset'>,
  storage?: Pick<Storage, 'getItem'>,
): {theme: ThemeMode; accent: AccentTheme} {
  const theme = normalizeThemeMode(storage?.getItem(themeStorageKey) ?? null)
  const accent = normalizeAccentTheme(storage?.getItem(accentStorageKey) ?? null)
  root.dataset.theme = theme
  root.dataset.accent = accent
  return {theme, accent}
}

export function setAppearance(
  root: Pick<HTMLElement, 'dataset'>,
  storage: Pick<Storage, 'setItem'>,
  theme: ThemeMode,
  accent: AccentTheme,
): void {
  root.dataset.theme = theme
  root.dataset.accent = accent
  storage.setItem(themeStorageKey, theme)
  storage.setItem(accentStorageKey, accent)
}

export function nextSidebarMode(mode: SidebarMode): SidebarMode {
  if (mode === 'full') return 'icon'
  if (mode === 'icon') return 'hidden'
  return 'full'
}
