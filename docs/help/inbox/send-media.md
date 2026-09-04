# Send media (image / video / document)

Attach an image, video, audio clip, document, or sticker to your reply. Nudgeway uploads the file and sends it in a single step from your point of view.

## How to use

1. Open the conversation in the middle pane.
2. Click the paperclip icon in the composer.
3. Pick a file. WhatsApp supports images, videos, audio, documents (PDF and others), and stickers. The maximum size is **16 MiB** — larger files aren't supported yet.
4. Optionally type a caption in the box that appears.
5. Click **Send**.

The bubble renders WhatsApp-native — images and video thumbnails inline, documents as a filename plus a download chip.

## Tips

- Videos must be H.264 with AAC audio in an MP4 container for WhatsApp to play them on the recipient's phone. Other codecs may upload but won't play. Convert first if you're unsure.
- If you're sending the same file to many contacts, upload it fresh each time — WhatsApp's own media IDs expire quickly.

## Troubleshooting

- **You see a red toast "File too large"** — your file is over 16 MiB. Compress the image or video, or split a large PDF.
- **You see "Meta couldn't fetch the file"** — your public tunnel (used only in local development) is down. If you're on a hosted deployment, refresh and try again; if the problem persists, an admin needs to check the tunnel.
- **The video plays with no sound, or won't play at all, on the recipient's phone** — the codec isn't H.264/AAC. Convert the video and re-send.
- **Upload spinner completes but the message shows "Send failed"** — the integration was disconnected while you were choosing the file. Refresh and try again; if it fails again, reconnect via [Integrations](#/integrations/connect-whatsapp).
- **The wrong file type icon appears on the recipient's side** — pick the file again and re-send. WhatsApp occasionally infers the wrong MIME type from the extension.

## Related

- [Send a text message](#/inbox/send-text) — the simpler path.
- [Send a template message](#/inbox/send-template) — media in template headers.
- [Troubleshooting](#/inbox/troubleshooting) — 24-hour window, real-time updates, attachment failures.
