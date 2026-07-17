/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Theme Tokens (Layer 3)
 *  Composed themes that combine raw + semantic values.
 *  Each theme is a complete set of CSS variable overrides.
 *
 *  Usage:
 *    import { lightTheme, compactTheme } from '@/ui-system/tokens/theme'
 * ═══════════════════════════════════════════════════════════════
 */

import { density } from './scales'
import type { DensityMode } from './scales'

/**
 * Light theme — the default WeExtension visual identity.
 * All CSS custom properties default to these values.
 */
export const lightTheme = {
  name: 'light' as const,

  brand: {
    primary:  '#2563eb',
    accent:   '#06b6d4',
    focus:    '#007bff',
  },

  text: {
    primary:   '#1e293b',
    secondary: '#475569',
    muted:     '#64748b',
    disabled:  '#94a3b8',
    inverse:   '#ffffff',
    link:      '#2563eb',
  },

  surface: {
    base:     '#f0f4f8',
    elevated: '#ffffff',
    sunken:   '#e2e8f0',
    overlay:  'rgba(15,23,42,0.3)',
  },

  border: {
    default:    '#cbd5e1',
    subtle:     '#e2e8f0',
    focus:      '#007bff',
    glass:      'rgba(255,255,255,0.7)',
    glassLight: 'rgba(255,255,255,0.4)',
  },

  scrollbar: {
    track:      '#e2e8f0',
    thumb:      '#94a3b8',
    thumbHover: '#64748b',
  },
} as const

/**
 * Returns density-specific overrides as CSS variable values.
 */
export function getDensityVars(mode: DensityMode) {
  const d = density[mode]
  return {
    '--wx-density-header-height':  d.headerHeight,
    '--wx-density-row-height':     d.rowHeight,
    '--wx-density-input-height':   d.inputHeight,
    '--wx-density-cell-px':        d.cellPaddingX,
    '--wx-density-cell-py':        d.cellPaddingY,
    '--wx-density-font-size':      d.fontSize,
    '--wx-density-icon-size':      d.iconSize,
    '--wx-density-avatar-size':    d.avatarSize,
    '--wx-density-gap':            d.gap,
    '--wx-density-container-pad':  d.containerPad,
  } as Record<string, string>
}

export type ThemeConfig = typeof lightTheme
