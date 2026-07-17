/**
 * ═══════════════════════════════════════════════════════════════
 *  WemakeUI — Typography numeric scale
 *
 *  Font: Inter (primary). Mono: ui-monospace.
 *  Sentence case Vietnamese — never uppercase except acronym.
 *
 *  CSS vars:
 *    --wx-fs-12 … --wx-fs-64
 *    --wx-lh-tight | snug | normal | relaxed
 *    --wx-fw-regular | medium | semibold | bold
 *    --wx-tracking-tight | normal | wide
 * ═══════════════════════════════════════════════════════════════
 */

export const fontSize = {
  12: '12px', /* caption, badge, meta, helper */
  13: '13px', /* secondary meta */
  14: '14px', /* body sm, label, button md, tab */
  15: '15px', /* body default */
  16: '16px', /* body lg, input lg */
  18: '18px', /* subtitle, h6 */
  20: '20px', /* h5 */
  24: '24px', /* h4 */
  28: '28px', /* h3 */
  32: '32px', /* h2 */
  40: '40px', /* h1 */
  48: '48px', /* hero */
  64: '64px', /* display */
} as const

export const lineHeight = {
  tight:   1.2,
  snug:    1.35,
  normal:  1.5,
  relaxed: 1.7,
} as const

export const fontWeight = {
  regular:  400,
  medium:   500,
  semibold: 600,
  bold:     700,
} as const

export const tracking = {
  tight:  '-0.01em',
  normal: '0',
  wide:   '0.02em',
} as const

export type FontSizeToken   = keyof typeof fontSize
export type LineHeightToken = keyof typeof lineHeight
export type FontWeightToken = keyof typeof fontWeight
export type TrackingToken   = keyof typeof tracking

export const fsVar       = (n: FontSizeToken):   string => `var(--wx-fs-${n})`
export const lhVar       = (k: LineHeightToken): string => `var(--wx-lh-${k})`
export const fwVar       = (k: FontWeightToken): string => `var(--wx-fw-${k})`
export const trackingVar = (k: TrackingToken):   string => `var(--wx-tracking-${k})`
