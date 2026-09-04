import { useEffect, useMemo, useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from './settings';
import { Spinner } from '../components/Spinner';
import { EmptyState } from '../components/EmptyState';
import { Button } from '../components/Button';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { ApiError } from '../lib/api';
import { useIntegrations } from '../lib/integrations';
import {
  TEMPLATE_STATUSES,
  isCreateWithSubmitError,
  useCreateTemplate,
  useDeleteTemplate,
  useSubmitTemplate,
  useSyncTemplates,
  useTemplates,
  useUpdateTemplate,
  type CreateTemplateInput,
  type Template,
  type TemplateCategory,
  type TemplateComponent,
  type TemplateListFilter,
  type TemplateStatus,
} from '../lib/templates';

// Curated top ~30 Meta locales — covers the majority of tenants. Full list
// lives in the Meta docs; this dropdown is a starter kit, not a whitelist.
const LANGUAGES: Array<{ code: string; label: string }> = [
  { code: 'en_US', label: 'English (US)' },
  { code: 'en_GB', label: 'English (UK)' },
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Spanish' },
  { code: 'es_ES', label: 'Spanish (Spain)' },
  { code: 'es_MX', label: 'Spanish (Mexico)' },
  { code: 'pt_BR', label: 'Portuguese (Brazil)' },
  { code: 'pt_PT', label: 'Portuguese (Portugal)' },
  { code: 'hi', label: 'Hindi' },
  { code: 'id', label: 'Indonesian' },
  { code: 'ar', label: 'Arabic' },
  { code: 'zh_CN', label: 'Chinese (Simplified)' },
  { code: 'zh_TW', label: 'Chinese (Traditional)' },
  { code: 'ja', label: 'Japanese' },
  { code: 'ko', label: 'Korean' },
  { code: 'fr', label: 'French' },
  { code: 'de', label: 'German' },
  { code: 'it', label: 'Italian' },
  { code: 'nl', label: 'Dutch' },
  { code: 'ru', label: 'Russian' },
  { code: 'tr', label: 'Turkish' },
  { code: 'vi', label: 'Vietnamese' },
  { code: 'th', label: 'Thai' },
  { code: 'ms', label: 'Malay' },
  { code: 'pl', label: 'Polish' },
  { code: 'ta', label: 'Tamil' },
  { code: 'te', label: 'Telugu' },
  { code: 'bn', label: 'Bengali' },
  { code: 'ur', label: 'Urdu' },
  { code: 'he', label: 'Hebrew' },
];

// Colour-coded status badge — mirrors the audit page pattern.
function StatusBadge({ status }: { status: TemplateStatus }) {
  const colours: Record<TemplateStatus, string> = {
    DRAFT: 'bg-slate-100 text-slate-700 ring-slate-200',
    PENDING: 'bg-amber-50 text-amber-700 ring-amber-100',
    APPROVED: 'bg-emerald-50 text-emerald-700 ring-emerald-100',
    REJECTED: 'bg-rose-50 text-rose-700 ring-rose-100',
    PAUSED: 'bg-orange-50 text-orange-700 ring-orange-100',
    DISABLED: 'bg-slate-100 text-slate-500 ring-slate-200',
  };
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${colours[status]}`}
    >
      {status}
    </span>
  );
}

// ─── Wizard state model ──────────────────────────────────────────────────

type HeaderKind = 'none' | 'text' | 'location';

type ButtonKind = 'QUICK_REPLY' | 'URL' | 'PHONE_NUMBER' | 'COPY_CODE' | 'VOICE_CALL';

type ButtonRow = {
  kind: ButtonKind;
  text: string;
  url: string;
  urlExample: string;
  phone: string;
  code: string;
};

type WizardState = {
  name: string;
  language: string;
  category: TemplateCategory;
  headerKind: HeaderKind;
  headerText: string;
  headerHasVar: boolean;
  headerVarSample: string;
  body: string;
  bodySamples: string[];
  bodyVarNames: string[]; // parallel to bodySamples; empty entry ⇒ positional
  footer: string;
  buttons: ButtonRow[];
  /** Adds the special {type:"call_permission_request"} component so this
   * template renders as a call-permission-request prompt in the WhatsApp
   * app. Meta requires the template category to be MARKETING or UTILITY
   * when this is set. See ~/Documents/whatsapp_doc_tracker/docs/calling/
   * user-call-permissions.md § "Using templates". */
  callPermissionRequest: boolean;
};

function emptyWizardState(): WizardState {
  return {
    name: '',
    language: 'en_US',
    category: 'MARKETING',
    headerKind: 'none',
    headerText: '',
    headerHasVar: false,
    headerVarSample: '',
    body: '',
    bodySamples: [],
    bodyVarNames: [],
    footer: '',
    buttons: [],
    callPermissionRequest: false,
  };
}

// Scan text for {{n}} or {{name}} tokens. Returns the ordered list of raw
// tokens as they appear (deduped by literal identity).
function scanVariables(text: string): string[] {
  const re = /\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g;
  const seen = new Set<string>();
  const out: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) {
    const key = m[1] ?? '';
    if (!seen.has(key)) {
      seen.add(key);
      out.push(key);
    }
  }
  return out;
}

// Build the Meta components[] array from wizard state.
function buildComponents(s: WizardState): TemplateComponent[] {
  const out: TemplateComponent[] = [];

  // Header.
  if (s.headerKind === 'text' && s.headerText.trim() !== '') {
    const comp: TemplateComponent = {
      type: 'HEADER',
      format: 'TEXT',
      text: s.headerText,
    };
    if (s.headerHasVar) {
      comp.example = { header_text: [s.headerVarSample] };
    }
    out.push(comp);
  } else if (s.headerKind === 'location') {
    out.push({ type: 'HEADER', format: 'LOCATION' });
  }

  // Body (required).
  const bodyVars = scanVariables(s.body);
  const bodyComp: TemplateComponent = { type: 'BODY', text: s.body };
  if (bodyVars.length > 0) {
    const samples = bodyVars.map((_, i) => s.bodySamples[i] ?? '');
    // If any variable is a name (not a digit), emit named-parameter example.
    const named = bodyVars.every((v) => /^[0-9]+$/.test(v)) === false;
    if (named) {
      bodyComp.example = {
        body_text_named_params: bodyVars.map((name, i) => ({
          param_name: name,
          example: samples[i] ?? '',
        })),
      };
    } else {
      bodyComp.example = { body_text: [samples] };
    }
  }
  out.push(bodyComp);

  // Footer.
  if (s.footer.trim() !== '') {
    out.push({ type: 'FOOTER', text: s.footer });
  }

  // Buttons.
  if (s.buttons.length > 0) {
    const btns: Array<Record<string, unknown>> = [];
    for (const b of s.buttons) {
      switch (b.kind) {
        case 'QUICK_REPLY':
          btns.push({ type: 'QUICK_REPLY', text: b.text });
          break;
        case 'URL': {
          const btn: Record<string, unknown> = { type: 'URL', text: b.text, url: b.url };
          if (scanVariables(b.url).length > 0 && b.urlExample.trim() !== '') {
            btn['example'] = [b.urlExample];
          }
          btns.push(btn);
          break;
        }
        case 'PHONE_NUMBER':
          btns.push({ type: 'PHONE_NUMBER', text: b.text, phone_number: b.phone });
          break;
        case 'COPY_CODE':
          btns.push({ type: 'COPY_CODE', example: b.code });
          break;
        case 'VOICE_CALL':
          btns.push({ type: 'VOICE_CALL', text: b.text });
          break;
      }
    }
    out.push({ type: 'BUTTONS', buttons: btns });
  }

  // Call permission request — Meta's special component. Renders the
  // in-app "Allow calls" prompt when the template message is delivered.
  // See user-call-permissions.md § "Using templates" for the shape.
  if (s.callPermissionRequest) {
    out.push({ type: 'call_permission_request' });
  }
  return out;
}

// Reverse: pull wizard state out of an existing template (best-effort).
function wizardFromTemplate(t: Template): WizardState {
  const s = emptyWizardState();
  s.name = t.name;
  s.language = t.language;
  s.category = t.category;
  for (const c of t.components) {
    const type = (c.type ?? '').toUpperCase();
    if (type === 'HEADER') {
      const fmt = (c.format ?? '').toUpperCase();
      if (fmt === 'TEXT') {
        s.headerKind = 'text';
        s.headerText = c.text ?? '';
        const vars = scanVariables(s.headerText);
        s.headerHasVar = vars.length > 0;
        const ex = c.example?.['header_text'];
        if (Array.isArray(ex) && ex.length > 0 && typeof ex[0] === 'string') {
          s.headerVarSample = ex[0];
        }
      } else if (fmt === 'LOCATION') {
        s.headerKind = 'location';
      }
    } else if (type === 'BODY') {
      s.body = c.text ?? '';
      const vars = scanVariables(s.body);
      s.bodyVarNames = vars;
      s.bodySamples = vars.map(() => '');
      const named = c.example?.['body_text_named_params'];
      if (Array.isArray(named)) {
        for (let i = 0; i < vars.length; i++) {
          const entry = named[i];
          if (entry !== null && typeof entry === 'object') {
            const ex = (entry as Record<string, unknown>)['example'];
            if (typeof ex === 'string') s.bodySamples[i] = ex;
          }
        }
      } else {
        const posit = c.example?.['body_text'];
        if (Array.isArray(posit) && Array.isArray(posit[0])) {
          const row = posit[0] as unknown[];
          for (let i = 0; i < vars.length; i++) {
            const v = row[i];
            if (typeof v === 'string') s.bodySamples[i] = v;
          }
        }
      }
    } else if (type === 'FOOTER') {
      s.footer = c.text ?? '';
    } else if (type === 'BUTTONS' && Array.isArray(c.buttons)) {
      for (const raw of c.buttons) {
        const bt = String(raw['type'] ?? '').toUpperCase() as ButtonKind;
        const row: ButtonRow = {
          kind: bt,
          text: String(raw['text'] ?? ''),
          url: String(raw['url'] ?? ''),
          urlExample: '',
          phone: String(raw['phone_number'] ?? ''),
          code: String(raw['example'] ?? ''),
        };
        const ex = raw['example'];
        if (Array.isArray(ex) && ex.length > 0 && typeof ex[0] === 'string') {
          row.urlExample = ex[0];
        }
        s.buttons.push(row);
      }
    } else if (type === 'CALL_PERMISSION_REQUEST') {
      // Meta's special component — case-insensitive match (the doc uses
      // lowercase but our uppercase normaliser strips that distinction).
      s.callPermissionRequest = true;
    }
  }
  return s;
}

// ─── Wizard UI ───────────────────────────────────────────────────────────

function TemplateWizard({
  open,
  mode,
  editing,
  whatsappIntegrations,
  defaultIntegrationID,
  onClose,
}: {
  open: boolean;
  mode: 'create' | 'edit';
  editing?: Template;
  whatsappIntegrations: Array<{ id: string; name: string }>;
  defaultIntegrationID: string;
  onClose: () => void;
}) {
  const [state, setState] = useState<WizardState>(() =>
    editing !== undefined ? wizardFromTemplate(editing) : emptyWizardState(),
  );
  const [integrationID, setIntegrationID] = useState<string>(
    editing?.integration_id ??
      (defaultIntegrationID !== '' ? defaultIntegrationID : (whatsappIntegrations[0]?.id ?? '')),
  );
  const [err, setErr] = useState<string | null>(null);
  const create = useCreateTemplate();
  const update = useUpdateTemplate();

  // Re-sync when we open with a different `editing` target.
  useEffect(() => {
    if (!open) return;
    setState(editing !== undefined ? wizardFromTemplate(editing) : emptyWizardState());
    setIntegrationID(
      editing?.integration_id ??
        (defaultIntegrationID !== ''
          ? defaultIntegrationID
          : (whatsappIntegrations[0]?.id ?? '')),
    );
    setErr(null);
    // We intentionally re-run every open to reset transient state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, editing?.id]);

  // Keep bodySamples length in sync with detected variables.
  useEffect(() => {
    const vars = scanVariables(state.body);
    setState((prev) => {
      if (
        prev.bodyVarNames.length === vars.length &&
        prev.bodyVarNames.every((n, i) => n === vars[i])
      ) {
        return prev;
      }
      const nextSamples = vars.map((_, i) => prev.bodySamples[i] ?? '');
      return { ...prev, bodyVarNames: vars, bodySamples: nextSamples };
    });
  }, [state.body]);

  if (!open) return null;

  const insertPositionalVar = () => {
    setState((prev) => {
      const vars = scanVariables(prev.body);
      const nums = vars
        .map((v) => Number.parseInt(v, 10))
        .filter((n) => Number.isFinite(n));
      const next = nums.length === 0 ? 1 : Math.max(...nums) + 1;
      return { ...prev, body: prev.body + `{{${next}}}` };
    });
  };

  const insertNamedVar = () => {
    const raw = window.prompt('Variable name (letters/digits/underscore)');
    if (raw === null) return;
    const name = raw.trim();
    if (name === '' || !/^[A-Za-z0-9_]+$/.test(name)) return;
    setState((prev) => ({ ...prev, body: prev.body + `{{${name}}}` }));
  };

  const addButton = () => {
    if (state.buttons.length >= 10) return;
    setState((prev) => ({
      ...prev,
      buttons: [
        ...prev.buttons,
        { kind: 'QUICK_REPLY', text: '', url: '', urlExample: '', phone: '', code: '' },
      ],
    }));
  };

  const updateButton = (idx: number, patch: Partial<ButtonRow>) => {
    setState((prev) => {
      const next = prev.buttons.map((b, i) => (i === idx ? { ...b, ...patch } : b));
      return { ...prev, buttons: next };
    });
  };

  const removeButton = (idx: number) => {
    setState((prev) => ({
      ...prev,
      buttons: prev.buttons.filter((_, i) => i !== idx),
    }));
  };

  const validate = (): string | null => {
    if (mode === 'create' && integrationID === '') return 'Pick an integration.';
    if (!/^[a-z0-9_]{1,512}$/.test(state.name)) {
      return 'Name must be lowercase letters, digits, or underscores.';
    }
    if (state.language === '') return 'Pick a language.';
    if (state.body.trim() === '') return 'Body is required.';
    if (state.body.length > 1024) return 'Body must be ≤1024 characters.';
    if (state.headerKind === 'text') {
      if (state.headerText.length > 60) return 'Header text must be ≤60 characters.';
      if (state.headerHasVar && state.headerVarSample.trim() === '') {
        return 'Header sample value required when a variable is present.';
      }
    }
    if (state.footer.length > 60) return 'Footer must be ≤60 characters.';
    for (let i = 0; i < state.bodyVarNames.length; i++) {
      if ((state.bodySamples[i] ?? '').trim() === '') {
        return `Sample value required for {{${state.bodyVarNames[i]}}}.`;
      }
    }
    for (let i = 0; i < state.buttons.length; i++) {
      const b = state.buttons[i]!;
      if (b.kind !== 'COPY_CODE' && (b.text.length === 0 || b.text.length > 25)) {
        return `Button ${i + 1}: label 1–25 chars required.`;
      }
      if (b.kind === 'URL') {
        if (b.url === '' || b.url.length > 2000) return `Button ${i + 1}: URL required (≤2000).`;
        if (scanVariables(b.url).length > 0 && b.urlExample.trim() === '') {
          return `Button ${i + 1}: URL example required when a variable is present.`;
        }
      }
      if (b.kind === 'PHONE_NUMBER' && (b.phone === '' || b.phone.length > 20)) {
        return `Button ${i + 1}: phone number required (≤20).`;
      }
      if (b.kind === 'COPY_CODE' && (b.code === '' || b.code.length > 15)) {
        return `Button ${i + 1}: code required (≤15).`;
      }
    }
    return null;
  };

  const submitCommon = async (submitToProvider: boolean) => {
    setErr(null);
    const v = validate();
    if (v !== null) {
      setErr(v);
      return;
    }
    const components = buildComponents(state);
    try {
      if (mode === 'edit' && editing !== undefined) {
        await update.mutateAsync({
          id: editing.id,
          category: state.category,
          components,
        });
        if (submitToProvider) {
          // Chain: run the update, then a separate submit call by parent.
          // Parent handles this by re-firing submit — here we just close.
        }
        onClose();
      } else {
        const input: CreateTemplateInput = {
          integration_id: integrationID,
          name: state.name,
          language: state.language,
          category: state.category,
          components,
          submit: submitToProvider,
        };
        const res = await create.mutateAsync(input);
        if (isCreateWithSubmitError(res)) {
          // DRAFT is saved but Meta rejected the submission — surface the
          // reason inline; leave the dialog open so the operator can fix
          // and try again.
          setErr(`Saved as DRAFT. Submission rejected: ${res.submit_error}`);
          return;
        }
        onClose();
      }
    } catch (e) {
      setErr(e instanceof ApiError ? (e.problem.detail ?? e.problem.title ?? 'Save failed') : 'Save failed');
    }
  };

  const busy = create.isPending || update.isPending;

  return (
    <div
      role="dialog"
      aria-modal="true"
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-900/40 p-4"
    >
      <form
        onSubmit={(e) => e.preventDefault()}
        className="my-8 w-full max-w-2xl space-y-5 rounded-2xl bg-white p-6 shadow-xl"
      >
        <div className="flex items-start justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900">
              {mode === 'edit' ? 'Edit template' : 'New template'}
            </h2>
            <p className="mt-1 text-xs text-slate-500">
              Drafted locally. Submit sends it to WhatsApp for review.
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

        {/* Integration + name + language + category */}
        <div className="grid gap-3 sm:grid-cols-2">
          <label className="block text-xs font-medium text-slate-600">
            Integration
            <select
              value={integrationID}
              onChange={(e) => setIntegrationID(e.target.value)}
              disabled={mode === 'edit'}
              className="mt-1 w-full rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800 disabled:bg-slate-100"
            >
              {whatsappIntegrations.length === 0 && <option value="">(none)</option>}
              {whatsappIntegrations.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block text-xs font-medium text-slate-600">
            Name
            <input
              required
              value={state.name}
              onChange={(e) => setState({ ...state, name: e.target.value })}
              disabled={mode === 'edit'}
              placeholder="order_confirmation"
              pattern="[a-z0-9_]+"
              className="mt-1 w-full rounded-lg border border-slate-200 px-2 py-1.5 font-mono text-sm text-slate-800 disabled:bg-slate-100"
            />
          </label>
          <label className="block text-xs font-medium text-slate-600">
            Language
            <select
              value={state.language}
              onChange={(e) => setState({ ...state, language: e.target.value })}
              disabled={mode === 'edit'}
              className="mt-1 w-full rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800 disabled:bg-slate-100"
            >
              {LANGUAGES.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.label} ({l.code})
                </option>
              ))}
            </select>
          </label>
          <fieldset className="block text-xs font-medium text-slate-600">
            <legend>Category</legend>
            <div className="mt-1 flex flex-col gap-1">
              {(['MARKETING', 'UTILITY', 'AUTHENTICATION'] as const).map((c) => (
                <label key={c} className="flex items-center gap-2 font-normal text-slate-700">
                  <input
                    type="radio"
                    name="cat"
                    checked={state.category === c}
                    onChange={() => setState({ ...state, category: c })}
                    disabled={c === 'AUTHENTICATION'}
                  />
                  {c}
                  {c === 'AUTHENTICATION' && (
                    <span className="text-[10px] text-slate-400">(coming soon)</span>
                  )}
                </label>
              ))}
            </div>
          </fieldset>
        </div>

        {/* Header */}
        <fieldset className="space-y-2 rounded-lg border border-slate-200 p-3">
          <legend className="px-1 text-xs font-medium text-slate-600">Header (optional)</legend>
          <div className="flex gap-4 text-xs text-slate-700">
            {(['none', 'text', 'location'] as const).map((k) => (
              <label key={k} className="flex items-center gap-1">
                <input
                  type="radio"
                  name="header-kind"
                  checked={state.headerKind === k}
                  onChange={() => setState({ ...state, headerKind: k })}
                />
                {k === 'none' ? 'None' : k[0]!.toUpperCase() + k.slice(1)}
              </label>
            ))}
            <span className="ml-auto text-[10px] text-slate-400">
              Media headers (image/video/document) — coming soon
            </span>
          </div>
          {state.headerKind === 'text' && (
            <div className="space-y-2">
              <input
                type="text"
                value={state.headerText}
                onChange={(e) => setState({ ...state, headerText: e.target.value })}
                maxLength={60}
                placeholder="Sale starts {{1}}!"
                className="w-full rounded-lg border border-slate-200 px-2 py-1.5 text-sm text-slate-800"
              />
              <label className="flex items-center gap-2 text-xs text-slate-700">
                <input
                  type="checkbox"
                  checked={state.headerHasVar}
                  onChange={(e) =>
                    setState({ ...state, headerHasVar: e.target.checked })
                  }
                />
                Header contains a variable
              </label>
              {state.headerHasVar && (
                <input
                  type="text"
                  value={state.headerVarSample}
                  onChange={(e) =>
                    setState({ ...state, headerVarSample: e.target.value })
                  }
                  placeholder="Sample value for variable"
                  className="w-full rounded-lg border border-slate-200 px-2 py-1.5 text-sm text-slate-800"
                />
              )}
            </div>
          )}
        </fieldset>

        {/* Body */}
        <fieldset className="space-y-2 rounded-lg border border-slate-200 p-3">
          <legend className="px-1 text-xs font-medium text-slate-600">Body (required)</legend>
          <textarea
            required
            value={state.body}
            onChange={(e) => setState({ ...state, body: e.target.value })}
            rows={4}
            maxLength={1024}
            placeholder="Hi {{1}}, your order {{2}} has shipped."
            className="w-full rounded-lg border border-slate-200 px-2 py-1.5 text-sm text-slate-800"
          />
          <div className="flex flex-wrap gap-2 text-xs">
            <button
              type="button"
              onClick={insertPositionalVar}
              className="rounded-md bg-slate-100 px-2 py-1 text-slate-700 hover:bg-slate-200"
            >
              + Insert {'{{n}}'}
            </button>
            <button
              type="button"
              onClick={insertNamedVar}
              className="rounded-md bg-slate-100 px-2 py-1 text-slate-700 hover:bg-slate-200"
            >
              + Insert named
            </button>
            <span className="ml-auto text-[10px] text-slate-400">{state.body.length}/1024</span>
          </div>
          {state.bodyVarNames.length > 0 && (
            <div className="space-y-1 pt-1">
              <p className="text-[10px] uppercase tracking-wide text-slate-500">Sample values</p>
              {state.bodyVarNames.map((name, i) => (
                <label key={i} className="flex items-center gap-2 text-xs text-slate-600">
                  <span className="w-16 font-mono">{`{{${name}}}`}</span>
                  <input
                    type="text"
                    value={state.bodySamples[i] ?? ''}
                    onChange={(e) => {
                      const next = state.bodySamples.slice();
                      next[i] = e.target.value;
                      setState({ ...state, bodySamples: next });
                    }}
                    placeholder="Sample value"
                    className="flex-1 rounded-lg border border-slate-200 px-2 py-1 text-sm text-slate-800"
                  />
                </label>
              ))}
            </div>
          )}
        </fieldset>

        {/* Footer */}
        <fieldset className="space-y-2 rounded-lg border border-slate-200 p-3">
          <legend className="px-1 text-xs font-medium text-slate-600">Footer (optional)</legend>
          <input
            type="text"
            value={state.footer}
            onChange={(e) => setState({ ...state, footer: e.target.value })}
            maxLength={60}
            placeholder="Reply STOP to opt-out"
            className="w-full rounded-lg border border-slate-200 px-2 py-1.5 text-sm text-slate-800"
          />
        </fieldset>

        {/* Buttons */}
        <fieldset className="space-y-2 rounded-lg border border-slate-200 p-3">
          <legend className="px-1 text-xs font-medium text-slate-600">
            Buttons (optional, up to 10)
          </legend>
          {state.buttons.length === 0 && (
            <p className="text-xs italic text-slate-400">No buttons.</p>
          )}
          {state.buttons.map((b, idx) => (
            <div
              key={idx}
              className="grid gap-2 rounded-lg border border-slate-100 p-2 sm:grid-cols-[110px,1fr,auto]"
            >
              <select
                value={b.kind}
                onChange={(e) => updateButton(idx, { kind: e.target.value as ButtonKind })}
                className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-xs text-slate-800"
              >
                <option value="QUICK_REPLY">Quick reply</option>
                <option value="URL">URL</option>
                <option value="PHONE_NUMBER">Phone number</option>
                <option value="COPY_CODE">Copy code</option>
                <option value="VOICE_CALL">Voice call</option>
              </select>
              <div className="flex flex-col gap-1">
                {b.kind !== 'COPY_CODE' && (
                  <input
                    type="text"
                    value={b.text}
                    onChange={(e) => updateButton(idx, { text: e.target.value })}
                    maxLength={25}
                    placeholder="Button label"
                    className="rounded-lg border border-slate-200 px-2 py-1 text-xs text-slate-800"
                  />
                )}
                {b.kind === 'URL' && (
                  <>
                    <input
                      type="url"
                      value={b.url}
                      onChange={(e) => updateButton(idx, { url: e.target.value })}
                      maxLength={2000}
                      placeholder="https://example.com/{{1}}"
                      className="rounded-lg border border-slate-200 px-2 py-1 text-xs text-slate-800"
                    />
                    {scanVariables(b.url).length > 0 && (
                      <input
                        type="text"
                        value={b.urlExample}
                        onChange={(e) => updateButton(idx, { urlExample: e.target.value })}
                        placeholder="https://example.com/summer (full expanded example)"
                        className="rounded-lg border border-slate-200 px-2 py-1 text-xs text-slate-800"
                      />
                    )}
                  </>
                )}
                {b.kind === 'PHONE_NUMBER' && (
                  <input
                    type="tel"
                    value={b.phone}
                    onChange={(e) => updateButton(idx, { phone: e.target.value })}
                    maxLength={20}
                    placeholder="+15551234567"
                    className="rounded-lg border border-slate-200 px-2 py-1 text-xs text-slate-800"
                  />
                )}
                {b.kind === 'COPY_CODE' && (
                  <input
                    type="text"
                    value={b.code}
                    onChange={(e) => updateButton(idx, { code: e.target.value })}
                    maxLength={15}
                    placeholder="Code to copy (e.g. SUMMER25)"
                    className="rounded-lg border border-slate-200 px-2 py-1 text-xs text-slate-800"
                  />
                )}
              </div>
              <button
                type="button"
                onClick={() => removeButton(idx)}
                className="self-start text-xs text-rose-600 hover:text-rose-800"
              >
                Remove
              </button>
            </div>
          ))}
          {state.buttons.length < 10 && (
            <button
              type="button"
              onClick={addButton}
              className="rounded-md bg-slate-100 px-2 py-1 text-xs text-slate-700 hover:bg-slate-200"
            >
              + Add button
            </button>
          )}
        </fieldset>

        <fieldset className="space-y-1 rounded-lg border border-slate-200 p-3">
          <legend className="px-1 text-xs font-medium uppercase tracking-wide text-slate-500">
            Special components
          </legend>
          <label className="flex items-start gap-2 text-sm text-slate-700">
            <input
              type="checkbox"
              checked={state.callPermissionRequest}
              onChange={(e) =>
                setState((s) => ({ ...s, callPermissionRequest: e.target.checked }))
              }
              className="mt-0.5 h-4 w-4 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500"
            />
            <span>
              <span className="font-medium">Call permission request</span>
              <span className="mt-0.5 block text-xs text-slate-500">
                Adds Meta's `call_permission_request` component so the recipient
                can grant your business permission to call them. Requires
                MARKETING or UTILITY category. See the Meta doc under
                <code className="mx-1 rounded bg-slate-100 px-1 py-0.5 text-[10px]">
                  user-call-permissions.md
                </code>
                § "Using templates".
              </span>
            </span>
          </label>
        </fieldset>

        {err !== null && (
          <p role="alert" className="text-xs text-rose-700">
            {err}
          </p>
        )}

        <div className="flex justify-end gap-2 pt-1">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            variant="secondary"
            onClick={() => {
              void submitCommon(false);
            }}
            loading={busy}
          >
            Save as draft
          </Button>
          {mode === 'create' && (
            <Button
              type="button"
              variant="primary"
              onClick={() => {
                void submitCommon(true);
              }}
              loading={busy}
            >
              Submit for review
            </Button>
          )}
        </div>
      </form>
    </div>
  );
}

function TemplateRow({
  t,
  onSubmit,
  onDelete,
  onEdit,
}: {
  t: Template;
  onSubmit: (id: string) => void;
  onDelete: (t: Template) => void;
  onEdit: (t: Template) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const canEdit = t.status === 'DRAFT';
  const canSubmit = t.status === 'DRAFT';
  return (
    <>
      <tr className="border-t border-slate-100 hover:bg-slate-50">
        <td className="px-3 py-2">
          <div className="font-medium text-slate-800">{t.name}</div>
          <div className="font-mono text-xs text-slate-500">{t.language}</div>
        </td>
        <td className="px-3 py-2 text-xs text-slate-600">{t.category}</td>
        <td className="px-3 py-2">
          <StatusBadge status={t.status} />
        </td>
        <td className="px-3 py-2 font-mono text-xs text-slate-500">
          {t.provider_template_id === undefined || t.provider_template_id === ''
            ? '—'
            : t.provider_template_id}
        </td>
        <td className="px-3 py-2 text-xs text-slate-500">
          {new Date(t.updated_at).toLocaleString()}
        </td>
        <td className="px-3 py-2 text-right">
          <div className="inline-flex items-center gap-2">
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              className="text-xs font-medium text-slate-600 hover:text-slate-900"
            >
              {expanded ? 'Hide' : 'View'}
            </button>
            <button
              type="button"
              onClick={() => onEdit(t)}
              disabled={!canEdit}
              className="text-xs font-medium text-slate-700 enabled:hover:text-slate-900 disabled:text-slate-300"
              title={canEdit ? 'Edit draft' : 'Only DRAFT rows are editable'}
            >
              Edit
            </button>
            <button
              type="button"
              onClick={() => onSubmit(t.id)}
              disabled={!canSubmit}
              className="text-xs font-medium text-emerald-700 enabled:hover:text-emerald-800 disabled:text-emerald-200"
            >
              Submit
            </button>
            <button
              type="button"
              onClick={() => onDelete(t)}
              className="text-xs font-medium text-rose-600 hover:text-rose-800"
            >
              Delete
            </button>
          </div>
        </td>
      </tr>
      {expanded && (
        <tr className="border-t border-slate-100 bg-slate-50">
          <td colSpan={6} className="px-3 py-3">
            <pre className="overflow-x-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">
              {JSON.stringify(t.components, null, 2)}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}

function TemplatesPage() {
  const integrations = useIntegrations();
  const whatsappIntegrations = useMemo(
    () =>
      (integrations.data ?? []).filter(
        (i) => i.provider === 'whatsapp' && i.status === 'connected',
      ),
    [integrations.data],
  );

  const [integrationID, setIntegrationID] = useState<string>('');

  // Default-select the first connected WhatsApp integration once loaded.
  useEffect(() => {
    if (integrationID === '' && whatsappIntegrations.length > 0) {
      setIntegrationID(whatsappIntegrations[0]!.id);
    }
  }, [whatsappIntegrations, integrationID]);

  const [status, setStatus] = useState<TemplateStatus | ''>('');
  const [wizardOpen, setWizardOpen] = useState(false);
  const [editing, setEditing] = useState<Template | undefined>(undefined);

  const filter: TemplateListFilter = useMemo(
    () => ({
      integration_id: integrationID,
      status,
      limit: 50,
    }),
    [integrationID, status],
  );

  const query = useTemplates(filter);
  const submit = useSubmitTemplate();
  const del = useDeleteTemplate();
  const sync = useSyncTemplates();

  const isPermDenied = query.error instanceof ApiError && query.error.status === 403;
  const isOffline = query.isError && typeof navigator !== 'undefined' && !navigator.onLine;

  const items = useMemo(() => {
    if (query.data === undefined) return [];
    const out: Template[] = [];
    for (const page of query.data.pages) out.push(...page.items);
    return out;
  }, [query.data]);

  const handleSync = () => {
    if (integrationID === '') return;
    sync.mutate({ integration_id: integrationID });
  };

  const [pendingDelete, setPendingDelete] = useState<Template | null>(null);

  const handleDelete = (t: Template) => {
    setPendingDelete(t);
  };

  const confirmDelete = () => {
    if (pendingDelete === null) return;
    const id = pendingDelete.id;
    del.mutate(id, {
      onSettled: () => setPendingDelete(null),
    });
  };

  const canCreate = whatsappIntegrations.length > 0;

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Message templates</h1>
          <p className="mt-1 text-sm text-slate-500">
            Draft, submit, and sync provider-side message templates. WhatsApp today; other
            channels follow.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="secondary"
            onClick={handleSync}
            disabled={integrationID === '' || sync.isPending}
          >
            {sync.isPending ? 'Syncing…' : 'Sync from provider'}
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              setEditing(undefined);
              setWizardOpen(true);
            }}
            disabled={!canCreate}
            title={canCreate ? '' : 'Connect a WhatsApp integration first.'}
          >
            New template
          </Button>
        </div>
      </header>

      <section
        aria-label="Filters"
        className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-3"
      >
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Integration
          <select
            value={integrationID}
            onChange={(e) => setIntegrationID(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            {whatsappIntegrations.length === 0 && <option value="">(none)</option>}
            {whatsappIntegrations.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Status
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value as TemplateStatus | '')}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {TEMPLATE_STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
      </section>

      {sync.error !== null && sync.error !== undefined && (
        <p role="alert" className="text-xs text-rose-700">
          Sync failed: {sync.error.problem.detail ?? sync.error.message}
        </p>
      )}
      {sync.data !== undefined && (
        <p className="text-xs text-emerald-700">
          Sync complete: fetched {sync.data.fetched}, upserted {sync.data.upserted}.
        </p>
      )}

      {query.isPending && (
        <div className="flex items-center justify-center rounded-xl border border-slate-200 bg-white py-12">
          <Spinner className="h-6 w-6 text-slate-500" label="Loading templates" />
        </div>
      )}

      {query.isError && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
        >
          <p className="font-medium">
            {isPermDenied
              ? "You don't have permission to view templates."
              : isOffline
                ? "You're offline. Reconnect to see templates."
                : 'Could not load templates.'}
          </p>
          {!isPermDenied && (
            <button
              type="button"
              onClick={() => void query.refetch()}
              className="mt-2 rounded-lg bg-white px-3 py-1 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
            >
              Retry
            </button>
          )}
        </div>
      )}

      {!query.isPending && !query.isError && items.length === 0 && (
        <EmptyState
          title="No templates yet."
          description={
            canCreate
              ? 'Press "New template" to draft one, or Sync to pull from the provider.'
              : 'Connect a WhatsApp integration first, then come back to draft templates.'
          }
        />
      )}

      {items.length > 0 && (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white">
          <table className="w-full text-left">
            <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-2 font-medium">Name / language</th>
                <th className="px-3 py-2 font-medium">Category</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Provider ID</th>
                <th className="px-3 py-2 font-medium">Updated</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {items.map((t) => (
                <TemplateRow
                  key={t.id}
                  t={t}
                  onSubmit={(id) => submit.mutate(id)}
                  onDelete={handleDelete}
                  onEdit={(row) => {
                    setEditing(row);
                    setWizardOpen(true);
                  }}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {query.hasNextPage === true && (
        <div className="flex justify-center">
          <Button
            variant="secondary"
            onClick={() => {
              void query.fetchNextPage();
            }}
            disabled={query.isFetchingNextPage}
          >
            {query.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}

      <TemplateWizard
        open={wizardOpen}
        mode={editing !== undefined ? 'edit' : 'create'}
        {...(editing !== undefined ? { editing } : {})}
        whatsappIntegrations={whatsappIntegrations.map((i) => ({ id: i.id, name: i.name }))}
        defaultIntegrationID={integrationID}
        onClose={() => {
          setWizardOpen(false);
          setEditing(undefined);
        }}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        title="Delete template"
        message={
          pendingDelete === null
            ? ''
            : `Delete template "${pendingDelete.name}"? This cannot be undone.`
        }
        confirmLabel="Delete"
        tone="danger"
        loading={del.isPending}
        onConfirm={confirmDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  );
}

export const settingsTemplatesRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/templates',
  component: TemplatesPage,
});
