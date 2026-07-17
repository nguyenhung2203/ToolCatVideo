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
    secondary: '#6b7280',
    muted:     '#9ca3af',
    disabled:  '#d1d5db',
    inverse:   '#ffffff',
    link:      '#2563eb',
  },

  surface: {
    base:     '#ffffff',
    elevated: '#ffffff',
    sunken:   '#f8fafc',
    overlay:  'rgba(0,0,0,0.2)',
  },

  border: {
    default:    '#e5e7eb',
    subtle:     '#f3f4f6',
    focus:      '#007bff',
    glass:      'rgba(255,255,255,0.5)',
    glassLight: 'rgba(255,255,255,0.3)',
  },

  scrollbar: {
    track:      '#f0f0f0',
    thumb:      '#b5b5b5',
    thumbHover: '#9a9a9a',
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
