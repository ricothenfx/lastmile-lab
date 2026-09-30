import type { Metadata, Viewport } from 'next';
import { Inter, JetBrains_Mono } from 'next/font/google';
import { tokens, cssVarsBlock } from '@/lib/tokens';
import './globals.css';

const inter = Inter({
  subsets: ['latin'],
  variable: '--font-ui',
  display: 'swap',
});
const mono = JetBrains_Mono({
  subsets: ['latin'],
  variable: '--font-mono',
  display: 'swap',
});

export const metadata: Metadata = {
  title: 'Pulse — Live Ops Map | lastmile-lab',
  description:
    'Control room untuk operasi last-mile: rider bergerak di jalan nyata Berlin, order streaming realtime, dispatch FIFO yang bisa dijelaskan.',
};

export const viewport: Viewport = {
  themeColor: tokens.color.bgBase,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${inter.variable} ${mono.variable}`}>
      <head>
        {/* Design tokens dari src/lib/tokens.ts — satu sumber kebenaran. */}
        <style dangerouslySetInnerHTML={{ __html: cssVarsBlock }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
