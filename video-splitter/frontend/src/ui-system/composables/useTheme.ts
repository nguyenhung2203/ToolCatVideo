/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Theme Composable
 *
 *  3 trục độc lập, có thể combine tự do:
 *    1. colorScheme — 'light' | 'dark' | 'system'     (legacy: theme)
 *    2. variant     — 'default' | 'flat'              (gradient on/off)
 *    3. brand/accent color — bất kỳ hex nào (consumer custom)
 *
 *  Usage:
 *    const {
 *      colorScheme, isDark, setColorScheme, toggleColorScheme,
 *      variant, setVariant,
 *      brandColor, setBrandColor,
 *      accentColor, setAccentColor,
 *      resetColors,
 *    } = useTheme()
 *
 *    setBrandColor('#8b5cf6')   // re-skin toàn UI sang tím
 *    setVariant('flat')         // tắt gradient
 *    setColorScheme('dark')     // dark mode
 *
 *  Legacy (backward-compat):
 *    const { theme, setTheme, toggleTheme } = useTheme()
 * ═══════════════════════════════════════════════════════════════
 */
import { ref, onMounted } from 'vue'
import { generateScale, applyScale } from '../utils/colorScale'

export type ColorScheme = 'light' | 'dark' | 'system'
export type Variant = 'default' | 'flat'

/** @deprecated dùng ColorScheme thay vì Theme. Giữ alias cho backward-compat. */
export type Theme = ColorScheme

const STORAGE_KEY = 'wx-theme'
const STORAGE_KEY_VARIANT = 'wx-variant'
const STORAGE_KEY_BRAND = 'wx-brand-color'
const STORAGE_KEY_ACCENT = 'wx-accent-color'

const colorScheme = ref<ColorScheme>('system')
const isDark = ref(false)
const variant = ref<Variant>('default')
const brandColor = ref<string | null>(null)
const accentColor = ref<string | null>(null)

function isBrowser(): boolean {
  return typeof window !== 'undefined' && typeof document !== 'undefined'
}

function getSystemPreference(): boolean {
  if (!isBrowser()) return false
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ?? false
}

function resolveColorScheme(t: ColorScheme): boolean {
  if (t === 'dark') return true
  if (t === 'light') return false
  return getSystemPreference()
}

function applyColorScheme(dark: boolean) {
  if (!isBrowser()) return
  isDark.value = dark
  const root = document.documentElement
  if (dark) {
    root.classList.add('wx-dark')
    root.setAttribute('data-wx-theme', 'dark')
  } else {
    root.classList.remove('wx-dark')
    root.setAttribute('data-wx-theme', 'light')
  }
}

function applyVariant(v: Variant) {
  if (!isBrowser()) return
  variant.value = v
  const root = document.documentElement
  root.classList.remove('wx-variant-flat')
  if (v === 'flat') {
    root.classList.add('wx-variant-flat')
    root.setAttribute('data-wx-variant', 'flat')
  } else {
    root.setAttribute('data-wx-variant', 'default')
  }
}

function applyBrand(hex: string | null, prefix: 'brand' | 'accent') {
  if (!isBrowser()) return
  const root = document.documentElement
  if (!hex) {
    // Clear inline overrides → quay lại scale default từ tokens.css
    for (const stop of ['50', '100', '200', '300', '400', '500', '600', '700', '800', '900']) {
      root.style.removeProperty(`--wx-${prefix}-${stop}`)
    }
    if (prefix === 'brand') {
      root.style.removeProperty('--wx-brand-primary')
      root.style.removeProperty('--wx-brand-focus')
    } else {
      root.style.removeProperty('--wx-brand-accent')
    }
    return
  }
  const scale = generateScale(hex)
  applyScale(scale, prefix, root)
  if (prefix === 'brand') {
    root.style.setProperty('--wx-brand-primary', scale['600'])
    root.style.setProperty('--wx-brand-focus', scale['500'])
  } else {
    root.style.setProperty('--wx-brand-accent', scale['500'])
  }
}

export function useTheme() {
  function setColorScheme(t: ColorScheme) {
    colorScheme.value = t
    if (isBrowser()) localStorage.setItem(STORAGE_KEY, t)
    applyColorScheme(resolveColorScheme(t))
  }

  function toggleColorScheme() {
    setColorScheme(isDark.value ? 'light' : 'dark')
  }

  function setVariant(v: Variant) {
    if (isBrowser()) localStorage.setItem(STORAGE_KEY_VARIANT, v)
    applyVariant(v)
  }

  function setBrandColor(hex: string | null) {
    brandColor.value = hex
    if (isBrowser()) {
      if (hex) localStorage.setItem(STORAGE_KEY_BRAND, hex)
      else localStorage.removeItem(STORAGE_KEY_BRAND)
    }
    applyBrand(hex, 'brand')
  }

  function setAccentColor(hex: string | null) {
    accentColor.value = hex
    if (isBrowser()) {
      if (hex) localStorage.setItem(STORAGE_KEY_ACCENT, hex)
      else localStorage.removeItem(STORAGE_KEY_ACCENT)
    }
    applyBrand(hex, 'accent')
  }

  function resetColors() {
    setBrandColor(null)
    setAccentColor(null)
  }

  onMounted(() => {
    if (!isBrowser()) return

    const storedScheme = localStorage.getItem(STORAGE_KEY) as ColorScheme | null
    if (storedScheme && ['light', 'dark', 'system'].includes(storedScheme)) {
      colorScheme.value = storedScheme
    }
    applyColorScheme(resolveColorScheme(colorScheme.value))

    const storedVariant = localStorage.getItem(STORAGE_KEY_VARIANT) as Variant | null
    if (storedVariant && ['default', 'flat'].includes(storedVariant)) {
      applyVariant(storedVariant)
    }

    const storedBrand = localStorage.getItem(STORAGE_KEY_BRAND)
    if (storedBrand) {
      brandColor.value = storedBrand
      applyBrand(storedBrand, 'brand')
    }
    const storedAccent = localStorage.getItem(STORAGE_KEY_ACCENT)
    if (storedAccent) {
      accentColor.value = storedAccent
      applyBrand(storedAccent, 'accent')
    }

    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    mq.addEventListener('change', (e) => {
      if (colorScheme.value === 'system') applyColorScheme(e.matches)
    })
  })

  return {
    // new API
    colorScheme,
    isDark,
    setColorScheme,
    toggleColorScheme,
    variant,
    setVariant,
    brandColor,
    setBrandColor,
    accentColor,
    setAccentColor,
    resetColors,

    // legacy aliases (backward-compat with old useTheme API)
    theme: colorScheme,
    setTheme: setColorScheme,
    toggleTheme: toggleColorScheme,
  }
}
