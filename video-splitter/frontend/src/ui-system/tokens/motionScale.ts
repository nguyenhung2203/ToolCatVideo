/**
 * ═══════════════════════════════════════════════════════════════
 *  WemakeUI — Motion tokens
 *
 *  CSS vars:
 *    --wx-d-instant | micro | fast | normal | slow | decorative
 *    --wx-ease-standard | decelerate | accelerate | bounce | spring
 *
 *  Use case map:
 *    micro       (100ms) — hover, focus ring
 *    fast        (150ms) — small swap, badge, page transition
 *    normal      (250ms) — modal, drawer, dropdown
 *    slow        (400ms) — section transition
 *    decorative  (700ms) — hero, illustration
 *
 *  Reduced motion: all durations collapse to 0ms via @media query
 *  in tokens.css.
 * ═══════════════════════════════════════════════════════════════
 */

export const duration = {
  instant:    '0ms',
  micro:      '100ms',
  fast:       '150ms',
  normal:     '250ms',
  slow:       '400ms',
  decorative: '700ms',
} as const

export const easing = {
  standard:   'cubic-bezier(0.4, 0, 0.2, 1)',
  decelerate: 'cubic-bezier(0, 0, 0.2, 1)',
  accelerate: 'cubic-bezier(0.4, 0, 1, 1)',
  bounce:     'cubic-bezier(0.16, 1, 0.3, 1)',
  spring:     'cubic-bezier(0.34, 1.56, 0.64, 1)',
} as const

export type DurationToken = keyof typeof duration
export type EasingToken   = keyof typeof easing

export const dVar    = (k: DurationToken): string => `var(--wx-d-${k})`
export const easeVar = (k: EasingToken):   string => `var(--wx-ease-${k})`

/** Compose a transition string: transition('opacity', 'normal', 'standard') */
export function transition(
  property: string = 'all',
  d: DurationToken = 'fast',
  e: EasingToken = 'standard',
): string {
  return `${property} ${duration[d]} ${easing[e]}`
}
