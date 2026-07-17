/**
 * ═══════════════════════════════════════════════════════════════
 *  WX UI SYSTEM — Design Tokens Public API
 *  Single entry point for all design tokens.
 *
 *  Usage:
 *    import { colors, semanticColors, radius, density } from '@/ui-system/tokens'
 * ═══════════════════════════════════════════════════════════════
 */

// ── Layer 1: Raw Tokens ────────────────────────────────
export { colors } from './colors'
export type { ColorToken } from './colors'

export { gradients } from './gradients'
export type { GradientToken } from './gradients'

export {
  radius,
  elevation,
  typography,
  spacing,
  motion,
  glass,
  zIndex,
  density,
} from './scales'
export type {
  RadiusToken,
  ElevationToken,
  TypographyToken,
  SpacingToken,
  MotionToken,
  GlassToken,
  ZIndexToken,
  DensityToken,
  DensityMode,
} from './scales'

// ── Layer 2: Semantic Tokens ───────────────────────────
export { semanticColors, semanticGradients } from './semantic'
export type { SemanticColorToken, SemanticGradientToken } from './semantic'

// ── Layer 3: Theme Tokens ──────────────────────────────
export { lightTheme, getDensityVars } from './theme'
export type { ThemeConfig } from './theme'

// ── Layer 4: Master spec scales (phase 0) ──────────────
export { space, spaceVar } from './spacing'
export type { SpaceToken } from './spacing'

export {
  fontSize,
  lineHeight,
  fontWeight,
  tracking,
  fsVar,
  lhVar,
  fwVar,
  trackingVar,
} from './typographyScale'
export type {
  FontSizeToken,
  LineHeightToken,
  FontWeightToken,
  TrackingToken,
} from './typographyScale'

export {
  duration,
  easing,
  dVar,
  easeVar,
  transition,
} from './motionScale'
export type { DurationToken, EasingToken } from './motionScale'

export { componentRadius, radiusOf, radiusVar } from './radiusMap'
export type { RadiusKey, ComponentRadiusKey } from './radiusMap'

export { componentShadow, shadowVar, shadowOf } from './elevationMap'
export type { ShadowKey, ComponentShadowKey } from './elevationMap'
