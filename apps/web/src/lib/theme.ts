'use client';

/**
 * Store tema (ADR D26) — dark default, light opsional. State = atribut
 * data-theme pada <html> (diset pre-paint oleh skrip init di layout.tsx).
 * CSS variables menangani seluruh UI Tailwind; objek token live (tokens.color)
 * disinkronkan saat modul ini dievaluasi di client (bundle deferred → skrip
 * pre-paint sudah jalan), lalu tiap ganti tema via setTheme. Konsumen
 * canvas/MapLibre (LiveMap) berlangganan untuk rebuild sprite/paint.
 */
import { applyPalette, type ThemeName } from './tokens';

export const THEME_KEY = 'pulse.theme';

type Listener = () => void;
const listeners = new Set<Listener>();

function current(): ThemeName {
  return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
}

// Sinkron awal (client-only): palet live mengikuti atribut yang sudah dipasang
// skrip pre-paint — sebelum draw canvas pertama di mana pun.
if (typeof document !== 'undefined') {
  applyPalette(current());
}

/** Set tema, persist pilihan user, dan notify pelanggan (canvas/map). */
export function setTheme(name: ThemeName, persist = true): void {
  document.documentElement.dataset.theme = name;
  document.documentElement.style.colorScheme = name;
  applyPalette(name);
  if (persist) {
    try {
      window.localStorage.setItem(THEME_KEY, name);
    } catch {
      /* storage diblokir — tema tetap berlaku untuk sesi ini */
    }
  }
  for (const l of listeners) l();
}

export function toggleTheme(): void {
  setTheme(current() === 'dark' ? 'light' : 'dark');
}

/**
 * Skrip inline pre-paint (ditempel di <head> oleh layout.tsx): set tema dari
 * localStorage → prefers-color-scheme → dark, SEBELUM paint pertama agar
 * tidak flash tema salah.
 */
export const initThemeScript = `(function(){try{var k=localStorage.getItem('${THEME_KEY}');var t=(k==='light'||k==='dark')?k:(window.matchMedia&&window.matchMedia('(prefers-color-scheme: light)').matches?'light':'dark');document.documentElement.dataset.theme=t;document.documentElement.style.colorScheme=t;}catch(e){document.documentElement.dataset.theme='dark';}})();`;

export function subscribeTheme(l: Listener): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

/** True saat tema light aktif (untuk composite canvas — lighter vs multiply). */
export function isLightTheme(): boolean {
  return current() === 'light';
}
