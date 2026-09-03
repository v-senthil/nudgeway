import { Modal } from '../../components/Modal';

type Props = {
  open: boolean;
  onClose: () => void;
  integration: string | null;
};

export function ComingSoonModal({ open, onClose, integration }: Props) {
  const title = integration !== null ? `Connect ${integration}` : 'Coming soon';
  return (
    <Modal open={open} onClose={onClose} title={title}>
      <p>
        Coming in Phase 1 — the integration wizard will land as part of the WhatsApp inbox
        milestone.
      </p>
    </Modal>
  );
}
