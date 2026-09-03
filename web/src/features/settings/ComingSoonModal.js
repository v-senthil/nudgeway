import { jsx as _jsx } from "react/jsx-runtime";
import { Modal } from '../../components/Modal';
export function ComingSoonModal({ open, onClose, integration }) {
    const title = integration !== null ? `Connect ${integration}` : 'Coming soon';
    return (_jsx(Modal, { open: open, onClose: onClose, title: title, children: _jsx("p", { children: "Coming in Phase 1 \u2014 the integration wizard will land as part of the WhatsApp inbox milestone." }) }));
}
