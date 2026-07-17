/**
 * ═══════════════════════════════════════════════════════════════
 *  WemakeUI — Spacing Tokens (4pt grid)
 *
 *  Scale: 0,4,8,12,16,24,32,40,48,64,80,96,128 (px)
 *  CSS vars: --wx-space-0 … --wx-space-12
 *
 *  Rule: every padding / margin / gap MUST be a multiple of 4px.
 *  Density mode (--wx-density-*) does NOT override this scale —
 *  density only changes row/cell/font/icon sizes.
 * ═══════════════════════════════════════════════════════════════
 */

export const space = {
  0:  '0',
  1:  '4px',
  2:  '8px',
  3:  '12px',
  4:  '16px',
  5:  '24px',
  6:  '32px',
  7:  '40px',
  8:  '48px',
  9:  '64px',
  10: '80px',
  11: '96px',
  12: '128px',
} as const

export type SpaceToken = keyof typeof space

/** CSS variable name for a space step. */
export const spaceVar = (step: SpaceToken): string => `var(--wx-space-${step})`
