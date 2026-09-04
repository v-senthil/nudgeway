import { useEffect, useState } from 'react';
import { Modal } from '../../components/Modal';
import { Button } from '../../components/Button';
import { Input } from '../../components/Input';
import { useCreateAPIToken, type CreateAPITokenResponse } from '../../lib/api-tokens';
import { ApiError } from '../../lib/api';

type Props = {
  open: boolean;
  onClose: () => void;
  /** Called with the plaintext-bearing response so the caller can
   * hand it to APITokenCreatedModal. */
  onCreated: (res: CreateAPITokenResponse) => void;
};

type ExpiryOption = { label: string; days: number | null };

const EXPIRY_OPTIONS: ExpiryOption[] = [
  { label: 'Never', days: null },
  { label: '7 days', days: 7 },
  { label: '30 days', days: 30 },
  { label: '90 days', days: 90 },
  { label: '1 year', days: 365 },
];

const MAX_NAME_LEN = 120;

/** CreateAPITokenModal collects a name + expiry and hands the
 * plaintext-bearing response back to the parent so it can show the
 * one-shot reveal modal. */
export function CreateAPITokenModal({ open, onClose, onCreated }: Props) {
  const [name, setName] = useState('');
  const [expiryIdx, setExpiryIdx] = useState(2); // default: 30 days
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useCreateAPIToken();

  useEffect(() => {
    if (!open) {
      setName('');
      setExpiryIdx(2);
      setNameError(undefined);
      setFormError(null);
      create.reset();
    }
  }, [open, create]);

  const trimmed = name.trim();
  const canSubmit = trimmed.length > 0 && trimmed.length <= MAX_NAME_LEN && !create.isPending;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setNameError(undefined);
    setFormError(null);

    if (trimmed.length === 0) {
      setNameError('Give the token a name so you can identify it later.');
      return;
    }
    if (trimmed.length > MAX_NAME_LEN) {
      setNameError(`Name must be ${MAX_NAME_LEN} characters or fewer.`);
      return;
    }

    const option = EXPIRY_OPTIONS[expiryIdx];
    const input =
      option !== undefined && option.days !== null
        ? { name: trimmed, expires_in_days: option.days }
        : { name: trimmed };

    try {
      const res = await create.mutateAsync(input);
      onCreated(res);
    } catch (err) {
      if (err instanceof ApiError) {
        const fieldErr = (err.problem.errors ?? []).find((e) => e.field === 'name');
        if (fieldErr?.message !== undefined) {
          setNameError(fieldErr.message);
        }
        setFormError(err.problem.detail ?? err.problem.title ?? 'Could not create token.');
      } else {
        setFormError('Unexpected error creating token.');
      }
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="New API token"
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={create.isPending}>
            Cancel
          </Button>
          <Button
            variant="primary"
            type="submit"
            form="create-api-token-form"
            loading={create.isPending}
            disabled={!canSubmit}
          >
            Create
          </Button>
        </>
      }
    >
      <form id="create-api-token-form" onSubmit={handleSubmit} className="space-y-3">
        <Input
          label="Name"
          placeholder="e.g. MCP server, CI deploy, personal laptop"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={nameError}
          maxLength={MAX_NAME_LEN}
          autoComplete="off"
          required
          hint="Where this token will be used. Purely for your own bookkeeping."
        />

        <label className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-slate-700">Expires in</span>
          <select
            value={expiryIdx}
            onChange={(e) => setExpiryIdx(Number.parseInt(e.target.value, 10))}
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 focus:outline-none focus:border-emerald-500 focus:ring-2 focus:ring-emerald-200"
          >
            {EXPIRY_OPTIONS.map((opt, idx) => (
              <option key={opt.label} value={idx}>
                {opt.label}
              </option>
            ))}
          </select>
          <p className="text-xs text-slate-500">
            Short lifetimes are safer. Rotate long-lived tokens on a schedule.
          </p>
        </label>

        {formError !== null && (
          <p
            role="alert"
            className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700 ring-1 ring-inset ring-rose-200"
          >
            {formError}
          </p>
        )}
      </form>
    </Modal>
  );
}
