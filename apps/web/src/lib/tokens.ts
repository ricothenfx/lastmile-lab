/**
 * Single source of truth design tokens "Pulse" (docs/DESIGN.md).
 * Komponen dilarang hardcode warna — semua lewat objek ini (canvas / style
 * MapLibre) atau CSS variables yang di-generate darinya (Tailwind / CSS).
 *
 * Dua tema (ADR D26): dark (default) & light. `tokens.color` adalah objek
 * LIVE — lib/theme.ts memutasi isinya saat tema berganti, jadi komponen yang
 * membacanya saat render/draw otomatis memakai palet aktif tanpa perlu tahu
 * soal tema. CSS variables di-inject untuk KEDUA tema (lihat cssVarsBlock).
 */
export interface Palette {
  bgBase: string;
  bgRaised: string;
  bgOverlay: string;
  borderSubtle: string;
  textPrimary: string;
  textSecondary: string;
  textMuted: string;
  accentCyan: string;
  accentLime: string;
  statusAmber: string;
  statusCoral: string;
  statusViolet: string;
  mapWater: string;
  mapRoad: string;
  mapRoadGlow: string;
  mapLabel: string;
  mapLabelMajor: string;
}

const themes = {
  dark: {
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
    // Peta: jalan dinaikkan kontrasnya (#223047 → #2B3B58) agar jalan terbaca
    // di zoom default — nama jalan kini fade-in mulai z12.2 (bukan z13).
    mapWater: '#0F1B2D',
    mapRoad: '#2B3B58',
    mapRoadGlow: '#35496E',
    mapLabel: '#94A3B8',
    mapLabelMajor: '#CBD5E1',
  },
  light: {
    bgBase: '#F8FAFC',
    bgRaised: '#FFFFFF',
    bgOverlay: '#F1F5F9',
    borderSubtle: '#E2E8F0',
    textPrimary: '#0F172A',
    textSecondary: '#475569',
    textMuted: '#64748B',
    // Aksen/status versi terang — hue sama dengan dark, digelapkan agar teks
    // berwarna tetap WCAG AA di surface terang (DESIGN.md §2).
    accentCyan: '#0E7490',
    accentLime: '#4D7C0F',
    statusAmber: '#B45309',
    statusCoral: '#BE123C',
    statusViolet: '#6D28D9',
    mapWater: '#C9E2F5',
    mapRoad: '#C4CFDB',
    mapRoadGlow: '#D8E0EA',
    mapLabel: '#5A6B80',
    mapLabelMajor: '#26334A',
  },
} satisfies Record<string, Palette>;

export type ThemeName = keyof typeof themes;
export { themes };

/** Palet aktif — objek LIVE, dimutasi lib/theme.ts saat tema berganti. */
export const tokens = {
  color: { ...themes.dark } as Palette,
  radius: {
    input: '4px',
    card: '8px',
    panel: '12px',
    pill: '999px',
  } as const,
  motion: {
    fastMs: 120,
    baseMs: 200,
    spring: { stiffness: 260, damping: 22 },
  } as const,
};

/** Rider status → warna token (urutan = model.RiderStatus di backend). LIVE. */
export const riderStatusColor: string[] = [
  tokens.color.accentLime, // 0 idle
  tokens.color.statusAmber, // 1 to_pickup
  tokens.color.accentCyan, // 2 pickup
  tokens.color.statusViolet, // 3 delivering
];

/** Terapkan palet ke objek live (dipanggil lib/theme.ts). */
export function applyPalette(name: ThemeName): void {
  const p = themes[name];
  for (const k of Object.keys(p) as (keyof Palette)[]) {
    tokens.color[k] = p[k];
  }
  riderStatusColor[0] = p.accentLime;
  riderStatusColor[1] = p.statusAmber;
  riderStatusColor[2] = p.accentCyan;
  riderStatusColor[3] = p.statusViolet;
}

/** Pemetaan token → nama CSS variable (kebab-case). */
const colorVars: Record<keyof Palette, string> = {
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
  mapRoadGlow: 'map-road-glow',
  mapLabel: 'map-label',
  mapLabelMajor: 'map-label-major',
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

function varsFor(p: Palette): string {
  return (Object.entries(colorVars) as [keyof Palette, string][])
    .map(([key, name]) => `--${name}:${p[key]};`)
    .join('');
}

/**
 * Blok CSS untuk layout — KEDUA tema sekaligus: dark jadi default (:root),
 * light aktif via [data-theme="light"] pada <html> (di-set pre-paint oleh
 * skrip init di layout.tsx; lihat lib/theme.ts).
 */
export const cssVarsBlock = `:root{${varsFor(themes.dark)}${Object.entries(radiusVars)
  .map(
    ([key, name]) =>
      `--${name}:${tokens.radius[key as keyof typeof tokens.radius]};`,
  )
  .join('')}${Object.entries(motionVars)
  .map(([key, name]) => `--${name}:${tokens.motion[key as keyof typeof tokens.motion]}ms;`)
  .join('')}}[data-theme="light"]{${varsFor(themes.light)}}`;
