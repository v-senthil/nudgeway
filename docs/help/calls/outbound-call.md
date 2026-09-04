# Outbound calls

Place a call from your side with a single click. The call queues, WhatsApp rings the recipient, and the resulting state transitions render as info messages in the thread just like inbound calls.

## Before you start

- The recipient must have granted you call permission — either **permanent** or **temporary** (unexpired). If they haven't, send a permission request first. See [Call permissions](#/calls/call-permissions).
- Your role must include call management. Ask an admin if the Call button is missing.
- The WhatsApp integration must be connected and healthy.

## How to use

1. Open the contact's conversation in the Inbox.
2. Look at the right pane. The **Call** button shows the current permission state:
   - **Green Call button** — permission is permanent or temporary and unexpired. You can call directly.
   - **Greyed-out Call button with a "Request permission" chip** — the recipient hasn't granted permission. Click **Request permission** first (see [Call permissions](#/calls/call-permissions)) and wait for them to tap Accept.
3. Click **Call**. The call queues, then rings the recipient.
4. When they answer, the live call bar appears at the top of the middle pane. Grant microphone permission if this is your first call.
5. Hang up with the red button in the call bar when you're done. An info message appears in the thread; click it to open the call detail page with the recording and transcript.

## Recording and transcript

Turn recording on for outbound calls in the call composer before you click Call — the toggle is next to the phone number. See [Recording and transcript](#/calls/recording-transcript) for how to access them afterwards.

## Troubleshooting

- **You see "Invalid phone number"** — the number contains spaces or a leading `+`. Enter digits only, in international format (for example `918197002143`, not `+91 8197 002143`).
- **"Permission denied — call management"** — your role doesn't allow placing calls. Ask an admin.
- **"Integration missing"** — the WhatsApp integration was disabled. Reconnect it in Settings → Integrations.
- **Call fails immediately with a "no permission" reason** — the recipient hasn't granted call permission. Click **Request permission** on the Call button and wait for them to tap Accept.
- **Call goes to "No answer" the instant it starts** — the recipient has WhatsApp calls disabled on their phone, or their phone is off. Try again later; there's nothing to fix on your side.
- **You can hear them but they can't hear you** — your microphone is blocked at the operating-system level. Grant mic access in System Settings.

## Related

- [Call permissions](#/calls/call-permissions) — check or request before you call.
- [Inbound calls](#/calls/inbound-call) — the popup and accept flow.
- [Recording and transcript](#/calls/recording-transcript) — post-call artefacts.
