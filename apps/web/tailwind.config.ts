import type { Config } from 'tailwindcss';

/**
 * Tailwind memetakan class → CSS variables yang di-generate dari
 * src/lib/tokens.ts (satu sumber kebenaran, docs/DESIGN.md).
 */
const config: Config = {
  content: ['./src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        surface: {
          base: 'var(--bg-base)',
          raised: 'var(--bg-raised)',
          overlay: 'var(--bg-overlay)',
        },
        line: {
          subtle: 'var(--border-subtle)',
        },
        ink: {
          primary: 'var(--text-primary)',
          secondary: 'var(--text-secondary)',
          muted: 'var(--text-muted)',
        },
        accent: {
          cyan: 'var(--accent-cyan)',
          lime: 'var(--accent-lime)',
        },
        status: {
          amber: 'var(--status-amber)',
          coral: 'var(--status-coral)',
          violet: 'var(--status-violet)',
        },
      },
      fontFamily: {
        ui: ['var(--font-ui)', 'system-ui', 'sans-serif'],
        mono: ['var(--font-mono)', 'ui-monospace', 'monospace'],
      },
      borderRadius: {
        input: 'var(--radius-input)',
        card: 'var(--radius-card)',
        panel: 'var(--radius-panel)',
        pill: 'var(--radius-pill)',
      },
      transitionDuration: {
        fast: 'var(--motion-fast)',
        base: 'var(--motion-base)',
      },
    },
  },
  plugins: [],
};

export default config;
