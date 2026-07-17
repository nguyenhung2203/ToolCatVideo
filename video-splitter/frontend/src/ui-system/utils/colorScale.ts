/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Color Scale Generator
 *
 *  Sinh ra dải 50→900 từ 1 hex color cho phép consumer truyền
 *  brand color tuỳ ý mà vẫn giữ cấu trúc scale (giống Tailwind).
 *
 *  Usage:
 *    const scale = generateScale('#8b5cf6')
 *    // → { '50': '#...', '100': '#...', ..., '900': '#...' }
 * ═══════════════════════════════════════════════════════════════
 */

export type ColorScale = Record<
  '50' | '100' | '200' | '300' | '400' | '500' | '600' | '700' | '800' | '900',
  string
>

interface HSL {
  h: number
  s: number
  l: number
}

/** Lightness targets cho từng stop. Mirror Tailwind v3 curve. */
const LIGHTNESS_STOPS: Record<keyof ColorScale, number> = {
  '50': 97,
  '100': 93,
  '200': 86,
  '300': 76,
  '400': 64,
  '500': 53,
  '600': 45,
  '700': 37,
  '800': 30,
  '900': 24,
}

function clamp(n: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, n))
}

function normalizeHex(hex: string): string {
  let h = hex.trim().replace(/^#/, '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  if (!/^[0-9a-fA-F]{6}$/.test(h)) {
    throw new Error(`[wx] Invalid hex color: "${hex}"`)
  }
  return h
}

function hexToRgb(hex: string): [number, number, number] {
  const h = normalizeHex(hex)
  return [
    parseInt(h.slice(0, 2), 16),
    parseInt(h.slice(2, 4), 16),
    parseInt(h.slice(4, 6), 16),
  ]
}

function rgbToHsl(r: number, g: number, b: number): HSL {
  const rn = r / 255
  const gn = g / 255
  const bn = b / 255
  const max = Math.max(rn, gn, bn)
  const min = Math.min(rn, gn, bn)
  const l = (max + min) / 2
  let h = 0
  let s = 0
  if (max !== min) {
    const d = max - min
    s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
    switch (max) {
      case rn: h = (gn - bn) / d + (gn < bn ? 6 : 0); break
      case gn: h = (bn - rn) / d + 2; break
      case bn: h = (rn - gn) / d + 4; break
    }
    h *= 60
  }
  return { h, s: s * 100, l: l * 100 }
}

function hslToHex(h: number, s: number, l: number): string {
  const sn = s / 100
  const ln = l / 100
  const c = (1 - Math.abs(2 * ln - 1)) * sn
  const hp = h / 60
  const x = c * (1 - Math.abs((hp % 2) - 1))
  let r = 0, g = 0, b = 0
  if (hp >= 0 && hp < 1) [r, g, b] = [c, x, 0]
  else if (hp < 2) [r, g, b] = [x, c, 0]
  else if (hp < 3) [r, g, b] = [0, c, x]
  else if (hp < 4) [r, g, b] = [0, x, c]
  else if (hp < 5) [r, g, b] = [x, 0, c]
  else [r, g, b] = [c, 0, x]
  const m = ln - c / 2
  const toHex = (v: number) => {
    const n = Math.round((v + m) * 255)
    return clamp(n, 0, 255).toString(16).padStart(2, '0')
  }
  return `#${toHex(r)}${toHex(g)}${toHex(b)}`
}

/**
 * Generate 10-step color scale từ 1 hex.
 * Giữ nguyên hue & saturation của input, scale lightness theo Tailwind curve.
 * Saturation giảm nhẹ ở 2 đầu (50/900) để tránh quá rực hoặc đục.
 */
export function generateScale(hex: string): ColorScale {
  const [r, g, b] = hexToRgb(hex)
  const { h, s } = rgbToHsl(r, g, b)
  const scale = {} as ColorScale
  for (const stop of Object.keys(LIGHTNESS_STOPS) as Array<keyof ColorScale>) {
    const l = LIGHTNESS_STOPS[stop]
    const adjustedS = l > 85 || l < 30 ? s * 0.85 : s
    scale[stop] = hslToHex(h, clamp(adjustedS, 0, 100), l)
  }
  return scale
}

/** Apply scale lên CSS vars dưới namespace --wx-{prefix}-{stop}. */
export function applyScale(
  scale: ColorScale,
  prefix: 'brand' | 'accent',
  target: HTMLElement = document.documentElement,
): void {
  for (const stop of Object.keys(scale) as Array<keyof ColorScale>) {
    target.style.setProperty(`--wx-${prefix}-${stop}`, scale[stop])
  }
}
