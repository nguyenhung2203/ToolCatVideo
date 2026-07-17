/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Semantic Tokens (Layer 2)
 *  Public consumption layer. Components import THESE, not raw.
 *
 *  import { semantic } from '@/ui-system/tokens'
 * ═══════════════════════════════════════════════════════════════
 */

import { colors } from './colors'
import { gradients } from './gradients'

// ── Semantic Colors ────────────────────────────────────
export const semanticColors = {
  brand: {
    primary:    colors.blue[600],     // #2563eb — main brand action
    accent:     colors.cyan[500],     // #06b6d4 — secondary accent
    focus:      colors.blue.brand,    // #007bff — focus rings, links
  },
  text: {
    primary:    colors.slate[800],    // #1e293b — headings, body
    secondary:  colors.gray[500],     // #6b7280 — descriptions
    muted:      colors.gray[400],     // #9ca3af — placeholders, timestamps
    disabled:   colors.gray[300],     // #d1d5db — disabled label
    inverse:    colors.white,         // white on gradients
    link:       colors.blue[600],     // #2563eb — hyperlinks
  },
  surface: {
    base:       colors.white,
    elevated:   colors.white,
    sunken:     colors.slate[50],     // #f8fafc — section backgrounds
    overlay:    'rgba(0,0,0,0.2)',    // modal backdrop
  },
  border: {
    default:    colors.gray[200],     // #e5e7eb — card/input borders
    subtle:     colors.gray[100],     // #f3f4f6 — dividers
    focus:      colors.blue.brand,    // #007bff — focus ring
    glass:      'rgba(255,255,255,0.5)',
    glassLight: 'rgba(255,255,255,0.3)',
  },
  success: { bg: colors.green[50],   text: colors.green[600],  solid: colors.green[500],  border: colors.green[100] },
  danger:  { bg: colors.red[50],     text: colors.red[600],    solid: colors.red[500],    border: colors.red[100] },
  warning: { bg: colors.amber[50],   text: colors.amber[600],  solid: colors.amber[500],  border: colors.amber[100] },
  info:    { bg: colors.blue[50],    text: colors.blue[600],   solid: colors.blue[500],   border: colors.blue[100] },
  neutral: { bg: colors.gray[50],    text: colors.gray[600],   solid: colors.gray[500],   border: colors.gray[200] },
  interactive: {
    selected:   { bg: '#ecf3fd', text: colors.blue.brand },    // paginator/table selected
    hover:      { bg: '#f6faff', text: '#6e98f1' },            // button hover
    active:     { bg: colors.blue[100], text: colors.blue[700] },
    disabled:   { bg: colors.gray[100], text: colors.gray[400] },
  },
} as const

// ── Semantic Gradients (public aliases) ───────────────
export const semanticGradients = {
  /** Use for page / app background */
  pageBg:         gradients.brandBg,
  /** Use for header bars, dialog headers */
  header:         gradients.brandHeader,
  /** Use for primary CTA buttons */
  cta:            gradients.brandCta,
  /** Use for secondary action buttons */
  button:         gradients.brandButton,
  /** Use for gradient text fills */
  textHighlight:  gradients.textAccent,
  /** Decorative divider fade */
  divider:        gradients.dividerFade,
} as const

export type SemanticColorToken    = typeof semanticColors
export type SemanticGradientToken = typeof semanticGradients
