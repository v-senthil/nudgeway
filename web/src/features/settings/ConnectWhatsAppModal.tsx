import { useEffect, useState } from 'react';
import { Modal } from '../../components/Modal';
import { Button } from '../../components/Button';
import { Input } from '../../components/Input';
import { useCreateIntegration } from '../../lib/integrations';
import type { Integration } from '../../lib/integrations';
import { ApiError } from '../../lib/api';

type Props = {
  open: boolean;
  onClose: () => void;
};

type Step = 'form' | 'webhook';

type FieldErrors = Partial<Record<'name' | 'phone_number_id' | 'waba_id' | 'access_token' | 'app_secret' | 'verify_token', string>>;

function extractFieldErrors(err: ApiError): FieldErrors {
  const out: FieldErrors = {};
  const arr = err.problem.errors ?? [];
  for (const e of arr) {
    if (e.field === undefined || e.message === undefined) continue;
    switch (e.field) {
      case 'name':
      case 'phone_number_id':
      case 'waba_id':
      case 'access_token':
      case 'app_secret':
      case 'verify_token':
        out[e.field] = e.message;
        break;
      default:
        break;
    }
  }
  return out;
}

export function ConnectWhatsAppModal({ open, onClose }: Props) {
  const [step, setStep] = useState<Step>('form');
  const [name, setName] = useState('');
  const [phoneNumberID, setPhoneNumberID] = useState('');
  const [wabaID, setWabaID] = useState('');
  const [accessToken, setAccessToken] = useState('');
  const [appSecret, setAppSecret] = useState('');
  const [verifyToken, setVerifyToken] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [created, setCreated] = useState<Integration | null>(null);
  const [copyMessage, setCopyMessage] = useState<string>('');

  const create = useCreateIntegration();

  useEffect(() => {
    if (!open) {
      // reset on close
      setStep('form');
      setName('');
      setPhoneNumberID('');
      setWabaID('');
      setAccessToken('');
      setAppSecret('');
      setVerifyToken('');
      setFieldErrors({});
      setFormError(null);
      setCreated(null);
      setCopyMessage('');
      create.reset();
    }
  }, [open, create]);

  const canSubmit =
    name.trim().length > 0 &&
    phoneNumberID.trim().length > 0 &&
    wabaID.trim().length > 0 &&
    accessToken.trim().length > 0 &&
    appSecret.trim().length > 0 &&
    verifyToken.trim().length > 0 &&
    !create.isPending;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setFieldErrors({});
    setFormError(null);
    try {
      const res = await create.mutateAsync({
        name: name.trim(),
        provider: 'whatsapp',
        phone_number_id: phoneNumberID.trim(),
        waba_id: wabaID.trim(),
        access_token: accessToken,
        app_secret: appSecret,
        verify_token: verifyToken,
      });
      setCreated(res);
      setStep('webhook');
    } catch (err) {
      if (err instanceof ApiError) {
        const fe = extractFieldErrors(err);
        if (Object.keys(fe).length > 0) setFieldErrors(fe);
        setFormError(err.problem.detail ?? err.problem.title ?? 'Failed to create integration');
      } else {
        setFormError('Unexpected error creating integration');
      }
    }
  };

  const copy = async (label: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopyMessage(`${label} copied to clipboard`);
    } catch {
      setCopyMessage(`Failed to copy ${label}`);
    }
    window.setTimeout(() => setCopyMessage(''), 2000);
  };

  const webhookURL = created?.webhook_url ?? `${window.location.origin}/webhooks/whatsapp/${created?.id ?? ''}`;
  const displayVerifyToken = created?.verify_token ?? verifyToken;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={step === 'form' ? 'Connect WhatsApp' : 'Set up Meta webhook'}
      footer={
        step === 'form' ? (
          <>
            <Button variant="secondary" onClick={onClose} disabled={create.isPending}>
              Cancel
            </Button>
            <Button
              variant="primary"
              onClick={(e) => {
                const form = (e.currentTarget as HTMLButtonElement).closest('form');
                if (form !== null) form.requestSubmit();
              }}
              loading={create.isPending}
              disabled={!canSubmit}
            >
              Connect
            </Button>
          </>
        ) : (
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        )
      }
    >
      {step === 'form' ? (
        <form onSubmit={handleSubmit} className="space-y-3">
          <Input
            label="Display name"
            placeholder="e.g. Support line"
            value={name}
            onChange={(e) => setName(e.target.value)}
            error={fieldErrors.name}
            autoComplete="off"
            required
          />
          <Input
            label="Phone number ID"
            placeholder="Meta phone_number_id"
            value={phoneNumberID}
            onChange={(e) => setPhoneNumberID(e.target.value)}
            error={fieldErrors.phone_number_id}
            autoComplete="off"
            required
          />
          <Input
            label="WhatsApp Business Account ID (WABA)"
            placeholder="waba_id"
            value={wabaID}
            onChange={(e) => setWabaID(e.target.value)}
            error={fieldErrors.waba_id}
            autoComplete="off"
            required
          />
          <Input
            label="Access token"
            type="password"
            placeholder="EAAG..."
            value={accessToken}
            onChange={(e) => setAccessToken(e.target.value)}
            error={fieldErrors.access_token}
            autoComplete="off"
            required
          />
          <Input
            label="App secret"
            type="password"
            placeholder="Meta app secret"
            value={appSecret}
            onChange={(e) => setAppSecret(e.target.value)}
            error={fieldErrors.app_secret}
            autoComplete="off"
            required
          />
          <Input
            label="Webhook verify token"
            placeholder="Choose any string; you'll paste this into Meta"
            value={verifyToken}
            onChange={(e) => setVerifyToken(e.target.value)}
            error={fieldErrors.verify_token}
            autoComplete="off"
            required
          />
          {formError !== null && (
            <p role="alert" className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700 ring-1 ring-inset ring-rose-200">
              {formError}
            </p>
          )}
        </form>
      ) : (
        <div className="space-y-4">
          <p className="text-sm text-slate-600">
            In the Meta App Dashboard, add a webhook to your WhatsApp Business account with the values below.
          </p>
          <CopyField label="Webhook URL" value={webhookURL} onCopy={copy} />
          <CopyField label="Verify token" value={displayVerifyToken} onCopy={copy} />
          <p className="text-xs text-slate-500">
            Subscribe to the <code className="rounded bg-slate-100 px-1">messages</code> field. fullWA will start
            receiving events immediately.
          </p>
          <div aria-live="polite" role="status" className="sr-only">
            {copyMessage}
          </div>
          {copyMessage.length > 0 && (
            <p className="text-xs text-emerald-700" aria-hidden="true">
              {copyMessage}
            </p>
          )}
        </div>
      )}
    </Modal>
  );
}

type CopyFieldProps = {
  label: string;
  value: string;
  onCopy: (label: string, value: string) => void;
};

function CopyField({ label, value, onCopy }: CopyFieldProps) {
  return (
    <div>
      <p className="text-xs font-medium text-slate-700">{label}</p>
      <div className="mt-1 flex items-center gap-2 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
        <code className="flex-1 truncate font-mono text-xs text-slate-800">{value}</code>
        <button
          type="button"
          onClick={() => onCopy(label, value)}
          aria-label={`Copy ${label}`}
          className="rounded-lg bg-white px-2 py-1 text-xs font-medium text-emerald-700 shadow-sm ring-1 ring-inset ring-slate-200 hover:bg-emerald-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500"
        >
          Copy
        </button>
      </div>
    </div>
  );
}
