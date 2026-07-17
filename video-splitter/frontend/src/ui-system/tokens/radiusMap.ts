/**
 * ═══════════════════════════════════════════════════════════════
 *  WemakeUI — Radius mapping per component (anatomy)
 *
 *  Hard rule: every component HAS a default radius.
 *  Never `border-radius: 0` for card / button / input / modal / dropdown.
 *
 *  Map: component → radius token (from --wx-radius-*).
 * ═══════════════════════════════════════════════════════════════
 */

import { radius } from './scales'

export type RadiusKey = keyof typeof radius

/** Default radius for every component in the system. */
export const componentRadius = {
  /* atom */
  button:        'md', /* 8px */
  input:         'md',
  textarea:      'md',
  select:        'md',
  checkbox:      'sm',
  radio:         'full',
  toggle:        'full',
  badge:         'full',
  avatar:        'full',
  chip:          'full',
  tag:           'sm',
  pill:          'full',
  spinner:       'full',
  progress:      'full',

  /* container */
  card:          'lg',  /* 12px */
  groupBox:      'lg',
  panel:         'lg',
  listItem:      'lg',
  segmented:     'lg',

  /* overlay */
  modal:         'xl',  /* 16px */
  drawer:        'xl',
  bottomSheet:   'xl',
  dropdown:      'md',
  popover:       'lg',
  tooltip:       'md',
  toast:         'md',
  contextMenu:   'md',

  /* feature */
  featureCard:   '2xl', /* 24px */
  pricingCard:   '2xl',
  hero:          '3xl', /* 30px */
  onboarding:    '4xl', /* 32px */

  /* skeleton variants */
  skeletonText:   'md',
  skeletonCard:   'lg',
  skeletonAvatar: 'full',
} as const satisfies Record<string, RadiusKey>

export type ComponentRadiusKey = keyof typeof componentRadius

/** Look up the radius token for a component. */
export function radiusOf(component: ComponentRadiusKey): string {
  return radius[componentRadius[component]]
}

/** CSS var form of a radius token. */
export const radiusVar = (k: RadiusKey): string => `var(--wx-radius-${k})`
