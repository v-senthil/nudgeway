import { Modal } from '../../components/Modal';
import { Button } from '../../components/Button';

type Props = {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  tokenName?: string | undefined;
  loading?: boolean;
  errorMessage?: string | undefined;
};

/** RevokeTokenModal is a scary-red confirm dialog for revoking an
 * API token. Revocation is instant and irreversible — any client
 * still using the value will start seeing 401s. */
export function RevokeTokenModal({
  open,
  onClose,
  onConfirm,
  tokenName,
  loading = false,
  errorMessage,
}: Props) {
  const title = tokenName !== undefined && tokenName !== '' ? `Revoke ${tokenName}?` : 'Revoke token?';
  return (
    <Modal
      open={open}
      onClose={onClose}
      title={title}
      footer={
        <>
          <Button variant="secondary" onClick={onClose} disabled={loading}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={onConfirm}
            loading={loading}
            className="bg-rose-600 hover:bg-rose-500 focus-visible:ring-rose-500"
          >
            Revoke
          </Button>
        </>
      }
    >
      <p className="text-sm text-slate-600">
        Any script, MCP server, or CI job using this token will start receiving 401 Unauthorized
        immediately. You can&rsquo;t undo this — mint a new token if you need access again.
      </p>
      {errorMessage !== undefined && errorMessage.length > 0 && (
        <p
          role="alert"
          className="mt-3 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700 ring-1 ring-inset ring-rose-200"
        >
          {errorMessage}
        </p>
      )}
    </Modal>
  );
}
