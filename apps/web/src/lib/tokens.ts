/**
 * Single source of truth design tokens "Pulse" (docs/DESIGN.md).
 * Komponen dilarang hardcode warna — semua lewat objek ini (canvas / style
 * MapLibre) atau CSS variables yang di-generate darinya (Tailwind / CSS).
 */
export const tokens = {
  color: {
    bgBase: '#0B1120',
    bgRaised: '#111827',
    bgOverlay: '#1F2937',
    borderSubtle: '#1E293B',
    textPrimary: '#F8FAFC',
    textSecondary: '#94A3B8',
    textMuted: '#64748B',
    accentCyan: '#22D3EE',
    accentLime: '#A3E635',
    statusAmber: '#FBBF24',
    statusCoral: '#FB7185',
    statusViolet: '#8B5CF6',
    mapWater: '#0F1B2D',
    mapRoad: '#223047',
  },
  radius: {
    input: '4px',
    card: '8px',
    panel: '12px',
    pill: '999px',
  },
  motion: {
    fastMs: 120,
    baseMs: 200,
    spring: { stiffness: 260, damping: 22 },
  },
} as const;

/** Rider status → warna token (urutan = model.RiderStatus di backend). */
export const riderStatusColor = [
  tokens.color.accentLime, // 0 idle
  tokens.color.statusAmber, // 1 to_pickup
  tokens.color.accentCyan, // 2 pickup
  tokens.color.statusViolet, // 3 delivering
] as const;

/** Pemetaan token → nama CSS variable (kebab-case). */
const colorVars: Record<keyof typeof tokens.color, string> = {
  bgBase: 'bg-base',
  bgRaised: 'bg-raised',
  bgOverlay: 'bg-overlay',
  borderSubtle: 'border-subtle',
  textPrimary: 'text-primary',
  textSecondary: 'text-secondary',
  textMuted: 'text-muted',
  accentCyan: 'accent-cyan',
  accentLime: 'accent-lime',
  statusAmber: 'status-amber',
  statusCoral: 'status-coral',
  statusViolet: 'status-violet',
  mapWater: 'map-water',
  mapRoad: 'map-road',
};

const radiusVars: Record<keyof typeof tokens.radius, string> = {
  input: 'radius-input',
  card: 'radius-card',
  panel: 'radius-panel',
  pill: 'radius-pill',
};

const motionVars: Record<string, string> = {
  fastMs: 'motion-fast',
  baseMs: 'motion-base',
};

/** Blok :root{} yang di-inject layout — satu sumber kebenaran untuk CSS. */
export const cssVarsBlock = `:root{${Object.entries(colorVars)
  .map(([key, name]) => `--${name}:${tokens.color[key as keyof typeof tokens.color]};`)
  .join('')}${Object.entries(radiusVars)
  .map(([key, name]) => `--${name}:${tokens.radius[key as keyof typeof tokens.radius]};`)
  .join('')}${Object.entries(motionVars)
  .map(([key, name]) => `--${name}:${tokens.motion[key as keyof typeof tokens.motion]}ms;`)
  .join('')}}`;
