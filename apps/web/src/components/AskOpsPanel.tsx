'use client';

/**
 * AskOpsPanel (Fase 7) — tab COPILOT di System Health. HANYA dirender bila
 * copilot enabled (pemanggil yang mengontrol). Jawaban backend WAJIB
 * bersitasi; jawaban tanpa sitasi ditolak service (422) dan panel hanya
 * menampilkan pesan penolakan — bukan isi jawabannya.
 */

import { useState } from 'react';
import { postCopilotAsk, type CopilotAnswer } from '@/lib/copilot';

export default function AskOpsPanel() {
  const [question, setQuestion] = useState('');
  const [phase, setPhase] = useState<'idle' | 'busy' | 'done' | 'error'>('idle');
  const [answer, setAnswer] = useState<CopilotAnswer | null>(null);
  const [errorMsg, setErrorMsg] = useState('');

  const ask = async () => {
    const q = question.trim();
    if (q.length < 3) return;
    setPhase('busy');
    setErrorMsg('');
    setAnswer(null);
    const out = await postCopilotAsk(q);
    if (out.error || !out.data) {
      setPhase('error');
      setErrorMsg(out.error ?? 'ask failed');
      return;
    }
    setAnswer(out.data);
    setPhase('done');
  };

  const busy = phase === 'busy';

  return (
    <div data-testid="copilot-ask">
      <p className="font-mono text-[9px] leading-relaxed text-ink-muted">
        ANSWERS MUST CITE INTERNAL DATA (KPI/INCIDENT) — UNCITED ANSWERS ARE REJECTED.
      </p>
      <div className="mt-1.5 grid grid-cols-[1fr_auto] gap-1.5">
        <input
          aria-label="Ops question"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !busy) void ask();
          }}
          placeholder="e.g. why is p95 spiking?"
          maxLength={500}
          className="rounded-input border border-line-subtle bg-surface-overlay px-2 py-1.5 font-mono text-[11px] text-ink-primary placeholder:text-ink-muted"
        />
        <button
          type="button"
          onClick={() => void ask()}
          disabled={busy || question.trim().length < 3}
          className="rounded-input border border-status-violet/60 bg-status-violet/10 px-3 py-1.5 font-mono text-[11px] tracking-[0.08em] text-status-violet transition-colors duration-fast hover:bg-status-violet/20 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {busy ? '…' : 'ASK'}
        </button>
      </div>

      {phase === 'error' && (
        <p className="mt-2 font-mono text-[10px] leading-relaxed text-status-coral" role="alert">
          {errorMsg.toUpperCase()}
        </p>
      )}

      {answer && (
        <div className="mt-2 rounded-card border border-line-subtle px-2 py-1.5" role="status">
          <p className="font-mono text-[10px] leading-relaxed text-ink-primary">{answer.text}</p>
          <p className="mt-1 font-mono text-[8px] uppercase tracking-[0.1em] text-ink-muted">
            Sources
          </p>
          <ul className="mt-0.5 space-y-0.5">
            {answer.sources.map((s) => (
              <li key={s.id} className="font-mono text-[9px] leading-relaxed">
                <span className="text-status-violet">{s.id}</span>
                <span className="text-ink-secondary"> — {s.label}: </span>
                <span className="tabular-nums text-ink-secondary">{s.value}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
