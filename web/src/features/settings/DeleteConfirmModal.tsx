import { Modal } from '../../components/Modal';
import { Button } from '../../components/Button';

type Props = {
  open: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title?: string;
  description?: string;
  confirmLabel?: string;
  loading?: boolean;
  errorMessage?: string | undefined;
};

export function DeleteConfirmModal({
  open,
  onClose,
  onConfirm,
  title = 'Delete integration',
  description = 'This will disconnect the channel and stop message delivery. This cannot be undone.',
  confirmLabel = 'Delete',
  loading = false,
  errorMessage,
}: Props) {
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
            {confirmLabel}
          </Button>
        </>
      }
    >
      <p className="text-sm text-slate-600">{description}</p>
      {errorMessage !== undefined && errorMessage.length > 0 && (
        <p role="alert" className="mt-3 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700 ring-1 ring-inset ring-rose-200">
          {errorMessage}
        </p>
      )}
    </Modal>
  );
}
