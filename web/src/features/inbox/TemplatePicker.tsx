import { useMemo, useState } from 'react';
import { Button } from '../../components/Button';
import { Spinner } from '../../components/Spinner';
import { useTemplates, type Template, type TemplateComponent } from '../../lib/templates';
import type { TemplateSendPayload } from '../../lib/messages';

// Regex for {{name}} or {{1}} placeholders — same as the wizard.
const VAR_RE = /\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g;

// One extracted variable slot the picker fills in.
type VarSlot = {
  key: string; // unique key for React
  label: string; // human-readable label (e.g. "Body {{1}}")
  sample: string; // sample value from template.example (placeholder)
  location: 'header' | 'body' | 'url_button';
  buttonIndex?: number;
};

// Walk a template's components → list of variables the operator must fill.
// Also captures the sample value from `example` blocks to use as placeholder.
function extractSlots(t: Template): VarSlot[] {
  const slots: VarSlot[] = [];
  let bi = 0;
  for (const c of t.components) {
    const type = (c.type ?? '').toUpperCase();
    if (type === 'HEADER' && (c.format ?? '').toUpperCase() === 'TEXT') {
      const vars = scan(c.text ?? '');
      if (vars.length > 0) {
        const sample = readSample(c.example?.['header_text'], 0);
        slots.push({
          key: 'h-0',
          label: `Header ${vars[0]!}`,
          sample,
          location: 'header',
        });
      }
    } else if (type === 'BODY') {
      const vars = scan(c.text ?? '');
      const named = Array.isArray(c.example?.['body_text_named_params']);
      for (let i = 0; i < vars.length; i++) {
        let sample = '';
        if (named) {
          const arr = c.example!['body_text_named_params'] as unknown[];
          const entry = arr[i];
          if (entry !== null && typeof entry === 'object') {
            const ex = (entry as Record<string, unknown>)['example'];
            if (typeof ex === 'string') sample = ex;
          }
        } else {
          const posit = c.example?.['body_text'];
          if (Array.isArray(posit) && Array.isArray(posit[0])) {
            const row = posit[0] as unknown[];
            const v = row[i];
            if (typeof v === 'string') sample = v;
          }
        }
        slots.push({
          key: `b-${i}`,
          label: `Body {{${vars[i]!}}}`,
          sample,
          location: 'body',
        });
      }
    } else if (type === 'BUTTONS' && Array.isArray(c.buttons)) {
      for (let i = 0; i < c.buttons.length; i++) {
        const b = c.buttons[i]!;
        if (String(b['type']).toUpperCase() === 'URL') {
          const urlVars = scan(String(b['url'] ?? ''));
          if (urlVars.length > 0) {
            let sample = '';
            const ex = b['example'];
            if (Array.isArray(ex) && ex.length > 0 && typeof ex[0] === 'string') {
              sample = ex[0];
            }
            slots.push({
              key: `btn-${bi}`,
              label: `URL button ${bi + 1}: ${b['text'] ?? ''}`,
              sample,
              location: 'url_button',
              buttonIndex: bi,
            });
          }
          bi++;
        } else if (String(b['type']).toUpperCase() === 'COPY_CODE') {
          // COPY_CODE at send time carries the code as a coupon_code parameter.
          slots.push({
            key: `btn-${bi}`,
            label: `Copy code button ${bi + 1}`,
            sample: String(b['example'] ?? ''),
            location: 'url_button',
            buttonIndex: bi,
          });
          bi++;
        } else {
          bi++;
        }
      }
    }
  }
  return slots;
}

function scan(text: string): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  let m: RegExpExecArray | null;
  const re = new RegExp(VAR_RE.source, 'g');
  while ((m = re.exec(text)) !== null) {
    const key = m[1] ?? '';
    if (!seen.has(key)) {
      seen.add(key);
      out.push(key);
    }
  }
  return out;
}

function readSample(example: unknown, idx: number): string {
  if (Array.isArray(example) && example.length > idx && typeof example[idx] === 'string') {
    return example[idx] as string;
  }
  return '';
}

// Build the send-time components[] block from a template + filled values.
function buildSendComponents(
  t: Template,
  values: Record<string, string>,
): TemplateSendPayload['components'] {
  const out: NonNullable<TemplateSendPayload['components']> = [];
  let bi = 0;

  // Header text parameter.
  const headerComp = t.components.find(
    (c) =>
      (c.type ?? '').toUpperCase() === 'HEADER' &&
      (c.format ?? '').toUpperCase() === 'TEXT',
  );
  if (headerComp !== undefined && scan(headerComp.text ?? '').length > 0) {
    out.push({
      type: 'header',
      parameters: [{ type: 'text', text: values['h-0'] ?? '' }],
    });
  }

  // Body parameters.
  const bodyComp = t.components.find((c) => (c.type ?? '').toUpperCase() === 'BODY');
  if (bodyComp !== undefined) {
    const vars = scan(bodyComp.text ?? '');
    if (vars.length > 0) {
      const named = Array.isArray(bodyComp.example?.['body_text_named_params']);
      const params: Array<Record<string, unknown>> = [];
      for (let i = 0; i < vars.length; i++) {
        const v = values[`b-${i}`] ?? '';
        if (named) {
          params.push({ type: 'text', parameter_name: vars[i], text: v });
        } else {
          params.push({ type: 'text', text: v });
        }
      }
      out.push({ type: 'body', parameters: params });
    }
  }

  // Buttons.
  const buttonsComp = t.components.find(
    (c) => (c.type ?? '').toUpperCase() === 'BUTTONS',
  );
  if (buttonsComp !== undefined && Array.isArray(buttonsComp.buttons)) {
    for (let i = 0; i < buttonsComp.buttons.length; i++) {
      const b = buttonsComp.buttons[i]!;
      const kind = String(b['type']).toUpperCase();
      if (kind === 'URL') {
        const urlVars = scan(String(b['url'] ?? ''));
        if (urlVars.length > 0) {
          out.push({
            type: 'button',
            sub_type: 'url',
            index: bi,
            parameters: [{ type: 'text', text: values[`btn-${bi}`] ?? '' }],
          });
        }
      } else if (kind === 'COPY_CODE') {
        out.push({
          type: 'button',
          sub_type: 'copy_code',
          index: bi,
          parameters: [{ type: 'coupon_code', coupon_code: values[`btn-${bi}`] ?? '' }],
        });
      }
      bi++;
    }
  }

  return out;
}

// ─── UI ────────────────────────────────────────────────────────────────

type Props = {
  onSend: (payload: TemplateSendPayload) => Promise<void> | void;
  onClose: () => void;
};

/** TemplatePicker — floating dialog that lets the operator pick an
 * APPROVED template, fill in parameter values, and send. */
export function TemplatePicker({ onSend, onClose }: Props) {
  const query = useTemplates({ status: 'APPROVED', limit: 100 });
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const items = useMemo(() => {
    if (query.data === undefined) return [];
    const out: Template[] = [];
    for (const page of query.data.pages) out.push(...page.items);
    return out;
  }, [query.data]);

  const selected = useMemo(
    () => items.find((t) => t.id === selectedID) ?? null,
    [items, selectedID],
  );
  const slots = useMemo(() => (selected === null ? [] : extractSlots(selected)), [selected]);

  const pick = (t: Template) => {
    setSelectedID(t.id);
    // Seed values with the template's sample defaults so a "just send"
    // click at least produces something coherent.
    const seeded: Record<string, string> = {};
    for (const s of extractSlots(t)) {
      seeded[s.key] = s.sample;
    }
    setValues(seeded);
    setErr(null);
  };

  const handleSend = async () => {
    if (selected === null) return;
    for (const s of slots) {
      if ((values[s.key] ?? '').trim() === '') {
        setErr(`Fill in "${s.label}" before sending.`);
        return;
      }
    }
    setBusy(true);
    setErr(null);
    try {
      const payload: TemplateSendPayload = {
        name: selected.name,
        language: selected.language,
      };
      const comps = buildSendComponents(selected, values);
      if (comps !== undefined && comps.length > 0) {
        payload.components = comps;
      }
      await onSend(payload);
      onClose();
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Send failed');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Send template"
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4"
    >
      <div className="flex max-h-[80vh] w-full max-w-2xl flex-col gap-3 rounded-2xl bg-white p-5 shadow-xl">
        <div className="flex items-start justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900">Send a template</h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Approved templates only. Fill in every variable before sending.
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-slate-400 hover:text-slate-700"
            aria-label="Close"
          >
            ×
          </button>
        </div>

        <div className="grid flex-1 gap-3 overflow-hidden sm:grid-cols-[260px,1fr]">
          <div className="flex flex-col gap-1 overflow-y-auto rounded-lg border border-slate-200 p-2">
            {query.isPending && (
              <div className="flex justify-center py-6">
                <Spinner className="h-5 w-5 text-slate-500" label="Loading templates" />
              </div>
            )}
            {query.isError && (
              <p className="rounded-md bg-rose-50 p-2 text-xs text-rose-800">
                Could not load templates.
              </p>
            )}
            {items.length === 0 && !query.isPending && !query.isError && (
              <p className="p-2 text-xs italic text-slate-400">
                No approved templates. Submit a draft and wait for Meta review.
              </p>
            )}
            {items.map((t) => (
              <button
                key={t.id}
                type="button"
                onClick={() => pick(t)}
                className={`flex flex-col items-start gap-0.5 rounded-md border px-2 py-1.5 text-left ${
                  t.id === selectedID
                    ? 'border-emerald-300 bg-emerald-50'
                    : 'border-transparent hover:border-slate-200 hover:bg-slate-50'
                }`}
              >
                <span className="text-sm font-medium text-slate-800">{t.name}</span>
                <span className="text-[10px] text-slate-500">
                  {t.language} · {t.category}
                </span>
              </button>
            ))}
          </div>

          <div className="flex flex-col gap-3 overflow-y-auto">
            {selected === null ? (
              <p className="text-sm italic text-slate-400">Pick a template from the list.</p>
            ) : (
              <>
                <div className="rounded-lg bg-slate-50 p-3 text-xs">
                  <p className="mb-1 font-medium text-slate-700">Preview</p>
                  <pre className="whitespace-pre-wrap font-sans text-slate-700">
                    {renderPreview(selected, values)}
                  </pre>
                </div>
                {slots.length === 0 ? (
                  <p className="rounded-lg bg-emerald-50 p-2 text-xs text-emerald-800">
                    No variables — press Send to fire.
                  </p>
                ) : (
                  <div className="flex flex-col gap-2">
                    <p className="text-xs font-medium text-slate-600">Fill variables</p>
                    {slots.map((s) => (
                      <label key={s.key} className="flex flex-col gap-1 text-xs text-slate-600">
                        {s.label}
                        <input
                          type="text"
                          value={values[s.key] ?? ''}
                          onChange={(e) =>
                            setValues((prev) => ({ ...prev, [s.key]: e.target.value }))
                          }
                          placeholder={s.sample !== '' ? s.sample : 'Value'}
                          className="rounded-lg border border-slate-200 px-2 py-1.5 text-sm text-slate-800"
                        />
                      </label>
                    ))}
                  </div>
                )}
              </>
            )}
          </div>
        </div>

        {err !== null && (
          <p role="alert" className="rounded-md bg-rose-50 px-2 py-1 text-xs text-rose-800">
            {err}
          </p>
        )}

        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              void handleSend();
            }}
            disabled={selected === null || busy}
            loading={busy}
          >
            Send
          </Button>
        </div>
      </div>
    </div>
  );
}

// Very small preview: substitute variables into header/body/footer text.
function renderPreview(t: Template, values: Record<string, string>): string {
  const parts: string[] = [];
  for (const c of t.components) {
    const type = (c.type ?? '').toUpperCase();
    if (type === 'HEADER' && (c.format ?? '').toUpperCase() === 'TEXT') {
      parts.push(substitute(c.text ?? '', () => values['h-0'] ?? ''));
    } else if (type === 'BODY') {
      parts.push(substitute(c.text ?? '', (i) => values[`b-${i}`] ?? ''));
    } else if (type === 'FOOTER') {
      parts.push(c.text ?? '');
    } else if (type === 'BUTTONS' && Array.isArray(c.buttons)) {
      const btns = c.buttons.map((b) => `[${String(b['text'] ?? 'button')}]`).join(' ');
      parts.push(btns);
    }
  }
  return parts.join('\n\n');
}

function substitute(text: string, get: (idx: number) => string): string {
  let out = text;
  const vars = scan(text);
  for (let i = 0; i < vars.length; i++) {
    const v = get(i);
    if (v === '') continue;
    out = out.replace(new RegExp(`\\{\\{\\s*${vars[i]!}\\s*\\}\\}`, 'g'), v);
  }
  return out;
}
