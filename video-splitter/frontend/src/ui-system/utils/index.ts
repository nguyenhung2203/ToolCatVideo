/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Utility Functions
 *  Pure helper functions with no side effects. No Vue reactivity.
 *
 *  Future exports:
 *    cn (class name merger), formatCurrency, formatDate
 *
 *  Usage:
 *    import { cn } from '@/ui-system/utils'
 * ═══════════════════════════════════════════════════════════════
 */

/**
 * Merge CSS class names, filtering out falsy values.
 * Lightweight alternative to clsx/classnames.
 *
 * @example
 *   cn('base', isActive && 'active', size === 'sm' && 'text-xs')
 *   // => 'base active text-xs' (if conditions are true)
 */
export function cn(...classes: (string | false | null | undefined)[]): string {
  return classes.filter(Boolean).join(' ')
}

export { generateScale, applyScale } from './colorScale'
export type { ColorScale } from './colorScale'
