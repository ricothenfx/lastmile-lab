import type { Metadata } from 'next';
import Link from 'next/link';
import { INTERVIEW_CATEGORIES, INTERVIEW_TOTAL } from '@/lib/interview';

export const metadata: Metadata = {
  title: 'Interview Q&A — Pulse · lastmile-lab',
  description: `Questions a senior interviewer would ask about this last-mile control room project — ${INTERVIEW_TOTAL} answers, each grounded in an ADR or a measured number from the repo.`,
};

export default function InterviewPage() {
  return (
    <main className="mx-auto min-h-dvh max-w-3xl bg-surface-base px-4 py-10 text-ink-primary sm:px-6">
      <p className="font-mono text-[10px] uppercase tracking-[0.12em] text-ink-secondary">
        <Link href="/" className="transition-colors duration-fast hover:text-accent-cyan">
          ← back to the control room
        </Link>
      </p>
      <h1 className="mt-4 font-mono text-[28px] font-bold leading-tight tracking-tight text-ink-primary">
        Interview Q&amp;A
      </h1>
      <p className="mt-3 max-w-2xl text-[14px] leading-relaxed text-ink-secondary">
        The questions a senior interviewer would ask about this project — written
        from the position of the examiner, not the candidate. {INTERVIEW_TOTAL}{' '}
        answers, each grounded in an architecture decision record or a measured
        number from the repo’s <span className="font-mono text-[12px]">reports/</span>.
        The weak spots are discussed openly; that is the point.
      </p>

      <nav aria-label="Categories" className="mt-6 flex flex-wrap gap-2">
        {INTERVIEW_CATEGORIES.map((c) => (
          <a
            key={c.title}
            href={`#${slug(c.title)}`}
            className="rounded-pill border border-line-subtle px-3 py-1 font-mono text-[10px] tracking-[0.06em] text-ink-secondary transition-colors duration-fast hover:border-accent-cyan/60 hover:text-accent-cyan"
          >
            {c.title} · {c.items.length}
          </a>
        ))}
      </nav>

      <div className="mt-10 space-y-10">
        {INTERVIEW_CATEGORIES.map((c) => (
          <section key={c.title} aria-labelledby={slug(c.title)}>
            <h2
              id={slug(c.title)}
              className="scroll-mt-6 border-b border-line-subtle pb-2 font-mono text-[13px] uppercase tracking-[0.12em] text-accent-cyan"
            >
              {c.title}
            </h2>
            <div className="mt-2 divide-y divide-line-subtle">
              {c.items.map((item) => (
                <details key={item.q} className="group py-3">
                  <summary className="cursor-pointer list-none marker:hidden">
                    <span className="mr-2 font-mono text-[12px] text-accent-cyan transition-transform duration-fast group-open:opacity-60">
                      ▸
                    </span>
                    <span className="text-[14px] font-medium leading-snug text-ink-primary">
                      {item.q}
                    </span>
                  </summary>
                  <p className="mt-2 pl-6 text-[13px] leading-relaxed text-ink-secondary">
                    {item.a}
                  </p>
                  <p className="mt-2 pl-6 font-mono text-[10px] tracking-[0.04em] text-ink-muted">
                    evidence: {item.ev}
                  </p>
                </details>
              ))}
            </div>
          </section>
        ))}
      </div>

      <footer className="mt-14 border-t border-line-subtle pt-4 font-mono text-[10px] leading-relaxed tracking-[0.04em] text-ink-muted">
        lastmile-lab · a simulated last-mile control room · every claim above links
        to an ADR (docs/BLUEPRINT.md §5) or a measured report (reports/)
      </footer>
    </main>
  );
}

function slug(title: string): string {
  return title.toLowerCase().replace(/[^a-z0-9]+/g, '-');
}
