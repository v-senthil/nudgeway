import { useState, type ReactNode } from 'react';
import {
  integrationPhoneNumberID,
  integrationWABAID,
  type Integration,
} from '../../lib/integrations';
import { usePhoneNumber, type PhoneNumber } from '../../lib/integration-settings';
import { PhoneNumberQRModal } from './PhoneNumberQRModal';
import { SetWebhookModal } from '../settings/SetWebhookModal';

/** DetailsTab is the read-only "at a glance" panel of the settings drawer.
 * Renders the WABA ID, Phone Number ID, and webhook URL with copy-to-
 * clipboard on every row — these are the values operators most often
 * need when configuring the Meta App dashboard. Also surfaces the live
 * Meta phone-number record (status, quality, tier, etc.) so operators
 * can spot issues without leaving Nudgeway. */
export function DetailsTab({ integration }: { integration: Integration }) {
  const pnid = integrationPhoneNumberID(integration);
  const waba = integrationWABAID(integration);
  const webhook = integration.webhook_url ?? '';
  const phoneNumberQuery = usePhoneNumber(integration.id);
  const [webhookModalOpen, setWebhookModalOpen] = useState(false);

  return (
    <div className="space-y-4">
      <p className="text-xs text-slate-500">
        Read-only reference values. Paste these into the Meta App dashboard when
        configuring your WhatsApp Business Account.
      </p>

      <DetailRow
        label="Phone Number ID"
        value={pnid}
        hint="Used as the sender id in every Meta send / call request."
      />
      <DetailRow
        label="WABA ID"
        value={waba}
        hint="WhatsApp Business Account id — scope for templates + groups."
      />
      <DetailRow
        label="Webhook URL"
        value={webhook}
        hint="Paste into Meta App dashboard → WhatsApp → Configuration → Webhook."
        action={
          <button
            type="button"
            onClick={() => setWebhookModalOpen(true)}
            className="rounded-md border border-emerald-200 bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-800 hover:bg-emerald-100"
          >
            Push to Meta
          </button>
        }
      />

      <SetWebhookModal
        open={webhookModalOpen}
        integration={integration}
        onClose={() => setWebhookModalOpen(false)}
      />

      <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-[11px] text-slate-600">
        <p className="font-medium text-slate-700">Integration ULID</p>
        <p className="mt-0.5 break-all font-mono text-slate-500">{integration.id}</p>
      </div>

      <PhoneNumberSection
        loading={phoneNumberQuery.isLoading}
        error={phoneNumberQuery.error?.problem?.detail ?? phoneNumberQuery.error?.message ?? null}
        phone={phoneNumberQuery.data ?? null}
      />
    </div>
  );
}

/** PhoneNumberSection renders the live Meta phone-number record. Empty
 * ({}) responses render as "no phone number returned" — the id is not
 * (yet) part of the WABA's phone-number list. Populated responses walk
 * a short, ordered list of fields with per-status colored pills. */
function PhoneNumberSection({
  loading,
  error,
  phone,
}: {
  loading: boolean;
  error: string | null;
  phone: PhoneNumber | null;
}) {
  const [qrOpen, setQrOpen] = useState(false);
  const populated =
    phone !== null &&
    (phone.display_phone_number !== undefined ||
      phone.verified_name !== undefined ||
      phone.status !== undefined);
  const digits =
    phone?.display_phone_number !== undefined
      ? phone.display_phone_number.replace(/[^0-9]/g, '')
      : '';

  return (
    <div className="rounded-lg border border-slate-200 bg-white">
      <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3">
        <h3 className="text-sm font-semibold text-slate-800">Phone number</h3>
        {loading && <Spinner />}
      </div>

      <div className="p-4">
        {error !== null ? (
          <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">
            {error}
          </div>
        ) : loading ? (
          <p className="text-xs text-slate-500">Loading Meta phone-number record…</p>
        ) : !populated ? (
          <p className="text-xs text-slate-500">
            No phone number returned for this integration. The configured Phone
            Number ID may not be part of this WABA yet.
          </p>
        ) : (
          <div className="space-y-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <p className="text-[11px] font-medium uppercase tracking-wide text-slate-500">
                  Display phone number
                </p>
                <p className="mt-1 text-xl font-bold text-slate-900">
                  {phone?.display_phone_number ?? ''}
                </p>
                {digits.length > 0 && <WaMeCopyChip digits={digits} />}
              </div>
              {digits.length > 0 && (
                <button
                  type="button"
                  onClick={() => setQrOpen(true)}
                  className="rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50"
                >
                  <span aria-hidden="true">📱</span> Show QR
                </button>
              )}
            </div>

            <dl className="grid grid-cols-1 gap-x-6 gap-y-3 sm:grid-cols-2">
              {phone?.verified_name !== undefined && (
                <Field label="Verified name">
                  <span className="text-sm text-slate-800">{phone.verified_name}</span>
                </Field>
              )}
              {phone?.status !== undefined && (
                <Field label="Status">
                  <StatusPill value={phone.status} />
                </Field>
              )}
              {phone?.quality_rating !== undefined && (
                <Field label="Quality rating">
                  <QualityPill value={phone.quality_rating} />
                </Field>
              )}
              {phone?.account_mode !== undefined && (
                <Field label="Account mode">
                  <AccountModePill value={phone.account_mode} />
                </Field>
              )}
              {phone?.messaging_limit_tier !== undefined && (
                <Field label="Messaging limit tier">
                  <span className="text-sm text-slate-800">{phone.messaging_limit_tier}</span>
                </Field>
              )}
              {(phone?.country_code !== undefined || phone?.country_dial_code !== undefined) && (
                <Field label="Country">
                  <span className="text-sm text-slate-800">
                    {phone?.country_code ?? ''}
                    {phone?.country_dial_code !== undefined && phone.country_dial_code.length > 0
                      ? ` +${phone.country_dial_code}`
                      : ''}
                  </span>
                </Field>
              )}
              {phone?.code_verification_status !== undefined && (
                <Field label="Code verification">
                  <CodeVerificationPill value={phone.code_verification_status} />
                </Field>
              )}
              {phone?.host_platform !== undefined && (
                <Field label="Host platform">
                  <span className="text-sm text-slate-800">{phone.host_platform}</span>
                </Field>
              )}
              {phone?.is_official_business_account !== undefined && (
                <Field label="Official Business Account">
                  <YesNoPill value={phone.is_official_business_account} />
                </Field>
              )}
            </dl>
          </div>
        )}
      </div>

      <PhoneNumberQRModal
        open={qrOpen}
        onClose={() => setQrOpen(false)}
        phone={digits}
      />
    </div>
  );
}

/** WaMeCopyChip is a tiny inline chip under the display phone number
 * that copies the corresponding wa.me deep link to the clipboard. */
function WaMeCopyChip({ digits }: { digits: string }) {
  const [copied, setCopied] = useState(false);
  const url = `wa.me/${digits}`;
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(`https://${url}`);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard may be blocked; the link is already visible.
    }
  };
  return (
    <button
      type="button"
      onClick={() => void copy()}
      className="mt-1 inline-flex items-center gap-1 rounded-full border border-emerald-200 bg-emerald-50 px-2 py-0.5 font-mono text-[11px] text-emerald-800 hover:bg-emerald-100"
      title="Copy wa.me link"
    >
      {copied ? 'Copied' : url}
    </button>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-[11px] font-medium uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className="mt-1">{children}</dd>
    </div>
  );
}

function Pill({
  className,
  children,
}: {
  className: string;
  children: ReactNode;
}) {
  return (
    <span
      className={
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ' + className
      }
    >
      {children}
    </span>
  );
}

function StatusPill({ value }: { value: string }) {
  const up = value.toUpperCase();
  const tone =
    up === 'CONNECTED'
      ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
      : up === 'PENDING'
        ? 'bg-amber-50 text-amber-700 border border-amber-200'
        : 'bg-slate-50 text-slate-700 border border-slate-200';
  return <Pill className={tone}>{value}</Pill>;
}

function QualityPill({ value }: { value: string }) {
  const up = value.toUpperCase();
  const tone =
    up === 'GREEN'
      ? 'bg-emerald-50 text-emerald-700 border border-emerald-200'
      : up === 'YELLOW'
        ? 'bg-amber-50 text-amber-700 border border-amber-200'
        : up === 'RED'
          ? 'bg-rose-50 text-rose-700 border border-rose-200'
          : 'bg-slate-50 text-slate-700 border border-slate-200';
  return <Pill className={tone}>{value}</Pill>;
}

function AccountModePill({ value }: { value: string }) {
  const up = value.toUpperCase();
  const tone =
    up === 'LIVE'
      ? 'bg-indigo-50 text-indigo-700 border border-indigo-200'
      : 'bg-slate-100 text-slate-700 border border-slate-200';
  return <Pill className={tone}>{value}</Pill>;
}

function CodeVerificationPill({ value }: { value: string }) {
  const up = value.toUpperCase();
  if (up === 'VERIFIED') {
    return (
      <Pill className="bg-emerald-50 text-emerald-700 border border-emerald-200">
        ✓ Verified
      </Pill>
    );
  }
  return (
    <Pill className="bg-rose-50 text-rose-700 border border-rose-200">✗ Not verified</Pill>
  );
}

function YesNoPill({ value }: { value: boolean }) {
  if (value) {
    return (
      <Pill className="bg-emerald-50 text-emerald-700 border border-emerald-200">Yes</Pill>
    );
  }
  return <Pill className="bg-slate-100 text-slate-700 border border-slate-200">No</Pill>;
}

/** Spinner is a small top-right loading affordance for the section
 * header — kept inline so the tab does not gain an external dep. */
function Spinner() {
  return (
    <span
      role="status"
      aria-label="Loading"
      className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-slate-300 border-t-emerald-600"
    />
  );
}

function DetailRow({
  label,
  value,
  hint,
  action,
}: {
  label: string;
  value: string | undefined;
  hint?: string;
  action?: ReactNode;
}) {
  const [copied, setCopied] = useState(false);
  const empty = value === undefined || value === '';

  const copy = async () => {
    if (empty) return;
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard may be blocked (insecure origin, permission denied) —
      // silently swallow; the value is already visible for manual copy.
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between">
        <label className="text-xs font-medium uppercase tracking-wide text-slate-500">
          {label}
        </label>
        <div className="flex items-center gap-3">
          {action}
          {!empty && (
            <button
              type="button"
              onClick={() => void copy()}
              className="text-xs font-medium text-emerald-700 hover:text-emerald-800"
            >
              {copied ? 'Copied' : 'Copy'}
            </button>
          )}
        </div>
      </div>
      <div
        className={
          'mt-1 flex items-center rounded-lg border px-3 py-2 font-mono text-sm ' +
          (empty
            ? 'border-slate-200 bg-slate-50 text-slate-400 italic'
            : 'border-slate-200 bg-white text-slate-800')
        }
      >
        <span className="break-all">{empty ? 'not configured' : value}</span>
      </div>
      {hint !== undefined && <p className="mt-1 text-[11px] text-slate-500">{hint}</p>}
    </div>
  );
}
