import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  footer?: React.ReactNode;
};

type ParameterLike = { text?: unknown; type?: unknown; [k: string]: unknown };
type ComponentLike = {
  type?: unknown;
  sub_type?: unknown;
  parameters?: unknown;
  text?: unknown;
  [k: string]: unknown;
};

type ResolvedButton = {
  type?: string;
  text?: string;
  url?: string;
  phone_number?: string;
  [k: string]: unknown;
};

type ResolvedShape = {
  header?: string;
  body?: string;
  footer?: string;
  buttons?: ResolvedButton[];
};

/**
 * TemplateBubble renders an outbound WhatsApp template message.
 *
 * Preferred path: the backend enriches the template metadata with a
 * `resolved` object carrying the header / body / footer text (with
 * `{{n}}` placeholders substituted from the send-time parameters) plus
 * the button list. When present we render the bubble the way WhatsApp
 * would render it on the recipient's phone.
 *
 * Fallback: older sends and any lookup misses keep the pre-enrichment
 * behaviour — a compact TEMPLATE label and a bulleted list of the raw
 * parameter values. That path also renders the raw JSON as a last
 * resort so operators never lose the underlying payload.
 */
export function TemplateBubble({ msg, footer }: Props) {
  const tpl = msg.template;
  if (tpl === undefined) {
    return (
      <div className="flex flex-col gap-1">
        <p className="text-[11px] uppercase tracking-wide opacity-70">Template</p>
        <p className="italic opacity-80">[template payload missing]</p>
        {footer}
      </div>
    );
  }

  const name = typeof tpl.name === 'string' ? tpl.name : '';
  const resolved = extractResolved(tpl.resolved);

  if (resolved !== null) {
    return (
      <div className="flex max-w-md flex-col gap-1.5">
        <p className="font-mono text-[10px] uppercase tracking-wide opacity-60">
          {'\u{1F4CB}'} TEMPLATE
          {name !== '' && <> · {name}</>}
        </p>
        {resolved.header !== undefined && resolved.header !== '' && (
          <p className="text-sm font-semibold break-words whitespace-pre-wrap">
            {resolved.header}
          </p>
        )}
        {resolved.body !== undefined && resolved.body !== '' && (
          <p className="break-words whitespace-pre-wrap">{resolved.body}</p>
        )}
        {resolved.footer !== undefined && resolved.footer !== '' && (
          <p className="text-[11px] opacity-70 break-words whitespace-pre-wrap">
            {resolved.footer}
          </p>
        )}
        {resolved.buttons !== undefined && resolved.buttons.length > 0 && (
          <div className="mt-1 flex flex-col gap-1">
            {resolved.buttons.map((b, i) => (
              <ResolvedButtonChip key={`${b.text ?? b.type ?? 'btn'}-${i}`} btn={b} />
            ))}
          </div>
        )}
        {footer}
      </div>
    );
  }

  // ---------- Fallback path (no resolved object) ----------
  const language = extractLanguageCode(tpl.language);
  const components = Array.isArray(tpl.components)
    ? (tpl.components as ComponentLike[])
    : [];

  const header = components.find((c) => strEq(c.type, 'header'));
  const body = components.find((c) => strEq(c.type, 'body'));
  const buttons = components.filter((c) => strEq(c.type, 'button'));

  const headerParams = paramTexts(header?.parameters);
  const bodyParams = paramTexts(body?.parameters);

  const nothingExtractable =
    components.length === 0 ||
    (headerParams.length === 0 && bodyParams.length === 0 && buttons.length === 0);

  return (
    <div className="flex flex-col gap-1.5">
      <p className="font-mono text-[11px] uppercase tracking-wide opacity-70">
        TEMPLATE
        {name !== '' && <> · {name}</>}
        {language !== '' && <> · {language}</>}
      </p>
      <p className="text-[11px] font-medium uppercase tracking-wide opacity-70">
        {'\u{1F4CB} Template'}
      </p>

      {headerParams.length > 0 && (
        <p className="text-xs opacity-80 break-words">{headerParams.join(' · ')}</p>
      )}

      {bodyParams.length > 0 && <p className="break-words">{bodyParams.join(' · ')}</p>}

      {buttons.length > 0 && (
        <div className="mt-1 flex flex-wrap gap-1.5">
          {buttons.map((b, i) => {
            const label = firstButtonLabel(b);
            return (
              <span
                key={`${label}-${i}`}
                className="inline-flex items-center rounded-full border border-white/40 bg-white/10 px-2 py-0.5 text-[11px] font-medium"
              >
                {label}
              </span>
            );
          })}
        </div>
      )}

      {nothingExtractable && (
        <pre className="mt-1 max-h-40 overflow-auto rounded-md bg-black/20 p-2 font-mono text-[10px] leading-tight">
          {JSON.stringify(tpl, null, 2)}
        </pre>
      )}

      {footer}
    </div>
  );
}

/**
 * ResolvedButtonChip renders one button from the resolved metadata as a
 * WhatsApp-style pill. Buttons are rendered but NOT clickable — they're
 * a preview of what the recipient sees on their phone, not a control
 * surface for the operator.
 */
function ResolvedButtonChip({ btn }: { btn: ResolvedButton }) {
  const rawType = typeof btn.type === 'string' ? btn.type.toUpperCase() : '';
  const label = typeof btn.text === 'string' && btn.text !== '' ? btn.text : rawType || 'BUTTON';
  return (
    <span className="inline-flex items-center justify-center gap-1.5 rounded-md border border-white/30 bg-white/10 px-3 py-1.5 text-xs font-medium">
      <ButtonTypeIcon type={rawType} />
      <span className="truncate">{label}</span>
    </span>
  );
}

function ButtonTypeIcon({ type }: { type: string }) {
  const props = {
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.75,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    className: 'h-3.5 w-3.5',
    'aria-hidden': true,
  };
  switch (type) {
    case 'URL':
      return (
        <svg {...props}>
          <path d="M10 14a5 5 0 0 0 7.07 0l3-3a5 5 0 0 0-7.07-7.07l-1.5 1.5" />
          <path d="M14 10a5 5 0 0 0-7.07 0l-3 3a5 5 0 0 0 7.07 7.07l1.5-1.5" />
        </svg>
      );
    case 'PHONE_NUMBER':
    case 'VOICE_CALL':
      return (
        <svg {...props}>
          <path d="M6.6 10.8a15 15 0 0 0 6.6 6.6l2.2-2.2a1 1 0 0 1 1-.24 11.4 11.4 0 0 0 3.6.57 1 1 0 0 1 1 1v3.4a1 1 0 0 1-1 1A17 17 0 0 1 3 4a1 1 0 0 1 1-1h3.4a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.6a1 1 0 0 1-.24 1L6.6 10.8Z" />
        </svg>
      );
    case 'COPY_CODE':
      return (
        <svg {...props}>
          <rect x="9" y="9" width="12" height="12" rx="2" />
          <path d="M5 15V5a2 2 0 0 1 2-2h10" />
        </svg>
      );
    case 'QUICK_REPLY':
    default:
      return null;
  }
}

/** extractResolved normalises the resolved blob into a typed shape. */
function extractResolved(raw: unknown): ResolvedShape | null {
  if (raw === null || raw === undefined || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  const out: ResolvedShape = {};
  if (typeof r.header === 'string') out.header = r.header;
  if (typeof r.body === 'string') out.body = r.body;
  if (typeof r.footer === 'string') out.footer = r.footer;
  if (Array.isArray(r.buttons)) {
    const btns: ResolvedButton[] = [];
    for (const item of r.buttons) {
      if (item === null || typeof item !== 'object') continue;
      const b = item as Record<string, unknown>;
      const btn: ResolvedButton = {};
      if (typeof b.type === 'string') btn.type = b.type;
      if (typeof b.text === 'string') btn.text = b.text;
      if (typeof b.url === 'string') btn.url = b.url;
      if (typeof b.phone_number === 'string') btn.phone_number = b.phone_number;
      btns.push(btn);
    }
    out.buttons = btns;
  }
  // Only surface a "resolved" bubble when at least one field landed —
  // otherwise fall back to the raw parameter list.
  if (
    out.header === undefined &&
    out.body === undefined &&
    out.footer === undefined &&
    (out.buttons === undefined || out.buttons.length === 0)
  ) {
    return null;
  }
  return out;
}

function strEq(v: unknown, target: string): boolean {
  return typeof v === 'string' && v.toLowerCase() === target;
}

function extractLanguageCode(lang: unknown): string {
  if (typeof lang === 'string') return lang;
  if (lang !== null && typeof lang === 'object' && 'code' in lang) {
    const code = (lang as { code?: unknown }).code;
    if (typeof code === 'string') return code;
  }
  return '';
}

function paramTexts(params: unknown): string[] {
  if (!Array.isArray(params)) return [];
  const out: string[] = [];
  for (const p of params as ParameterLike[]) {
    if (p === null || typeof p !== 'object') continue;
    if (typeof p.text === 'string' && p.text !== '') {
      out.push(p.text);
      continue;
    }
    // Media params carry {image:{link},...} etc. Surface a compact hint
    // so the operator still sees something.
    if (typeof p.type === 'string' && p.type !== '' && p.type !== 'text') {
      out.push(`<${p.type}>`);
    }
  }
  return out;
}

function firstButtonLabel(c: ComponentLike): string {
  if (typeof c.text === 'string' && c.text !== '') return c.text;
  const params = c.parameters;
  if (Array.isArray(params)) {
    for (const p of params as ParameterLike[]) {
      if (typeof p?.text === 'string' && p.text !== '') return p.text;
    }
  }
  const sub = typeof c.sub_type === 'string' ? c.sub_type : 'button';
  return sub;
}
