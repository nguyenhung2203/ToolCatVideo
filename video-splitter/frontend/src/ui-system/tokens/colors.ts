/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Raw Color Tokens (Layer 1)
 *  Pure color scales. No semantic meaning.
 *  NEVER import these directly in components — use semantic.ts.
 * ═══════════════════════════════════════════════════════════════
 */

export const colors = {
  // ── Brand Cyan Spectrum ──────────────────────────────
  cyan: {
    50:  '#ecfeff',
    100: '#cffafe',
    200: '#a5f3fc',
    300: '#67e8f9',
    400: '#22d3ee',
    500: '#06b6d4',
    600: '#0891b2',
    700: '#0e7490',
    /** App.vue background start stop */
    light:  '#7ecff1',
    /** App.vue background mid stop */
    mid:    '#9cc9f5',
    /** App.vue background end stop */
    bright: '#95def5',
  },

  // ── Brand Blue Spectrum ──────────────────────────────
  blue: {
    50:  '#eff6ff',
    100: '#dbeafe',
    200: '#bfdbfe',
    300: '#93c5fd',
    400: '#60a5fa',
    500: '#3b82f6',
    600: '#2563eb',
    700: '#1d4ed8',
    800: '#1e40af',
    900: '#1e3a8a',
    /** global.css focus/link blue */
    brand: '#007bff',
    /** Login CTA gradient end */
    dark: '#0052D4',
    /** Login CTA gradient mid */
    cta: '#3a7bd5',
    /** Setting sidebar active */
    link: '#0059b0',
  },

  // ── Neutral: Slate (cool gray) ──────────────────────
  slate: {
    50:  '#f8fafc',
    100: '#f1f5f9',
    200: '#e2e8f0',
    300: '#cbd5e1',
    400: '#94a3b8',
    500: '#64748b',
    600: '#475569',
    700: '#334155',
    800: '#1e293b',
    900: '#0f172a',
    950: '#020617',
  },

  // ── Neutral: Gray (warm gray) ───────────────────────
  gray: {
    50:  '#f9fafb',
    100: '#f3f4f6',
    200: '#e5e7eb',
    300: '#d1d5db',
    400: '#9ca3af',
    500: '#6b7280',
    600: '#4b5563',
    700: '#374151',
    800: '#1f2937',
    900: '#111827',
  },

  // ── Semantic Hues ───────────────────────────────────
  green: {
    50:  '#f0fdf4',
    100: '#dcfce7',
    500: '#22c55e',
    600: '#16a34a',
    700: '#15803d',
  },
  red: {
    50:  '#fef2f2',
    100: '#fee2e2',
    500: '#ef4444',
    600: '#dc2626',
    700: '#b91c1c',
  },
  amber: {
    50:  '#fffbeb',
    100: '#fef3c7',
    200: '#fde68a',
    400: '#fbbf24',
    500: '#f59e0b',
    600: '#d97706',
  },
  orange: {
    400: '#fb923c',
    500: '#f97316',
  },
  rose: {
    500: '#f43f5e',
    600: '#e11d48',
  },
  emerald: {
    500: '#10b981',
    600: '#059669',
  },
  violet: {
    500: '#8b5cf6',
    600: '#7c3aed',
  },

  // ── Base ────────────────────────────────────────────
  white:       '#ffffff',
  black:       '#000000',
  transparent: 'transparent',
} as const

export type ColorToken = typeof colors
