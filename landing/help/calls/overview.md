# Calls overview

WhatsApp Business Calling is a first-class channel in Nudgeway. Inbound calls appear as a bottom-right popup with Accept and Reject buttons; accepting hosts the call directly in your browser tab. Every call state change is mirrored into the conversation thread as an info message (ringing, answered, completed), and the terminal message links through to the call detail page with the recording and transcript.

## What a call looks like

- A call belongs to one integration, one contact, and one conversation.
- Direction is either **inbound** (customer called you) or **outbound** (you called the customer).
- Status moves through: queued, ringing, answered, in progress, completed — or ends in missed, failed, declined, or no answer.
- Recording and transcript, when enabled, become available on the call detail page a few seconds to a minute after the call ends.

## Calls happen in your browser

When you accept a call, your browser hosts the audio session directly. That means:

- You need to grant microphone permission the first time. Click the padlock in the address bar and allow microphone access.
- The tab needs to stay open while the call is live. Closing the tab ends the call.
- Calls require an HTTPS URL to work. Local development on `localhost` works too.

## Permissions

Before you call a WhatsApp user, they must have granted you call permission — either **permanent** or **temporary** (unexpired). If they haven't, send them an interactive permission request first and wait for them to tap Accept. See [Call permissions](#/calls/call-permissions).

## What you can do here

| Task | Page |
|---|---|
| Accept an inbound call | [Inbound calls](#/calls/inbound-call) |
| Place an outbound call | [Outbound calls](#/calls/outbound-call) |
| Check or request call permission | [Call permissions](#/calls/call-permissions) |
| Access recording and transcript | [Recording and transcript](#/calls/recording-transcript) |
| Common failure modes | [Troubleshooting](#/calls/troubleshooting) |

## Related

- [Inbox overview](#/inbox/overview) — call transitions appear inline in the thread.
- [Call settings](#/integrations/call-settings) — per-integration call hours, callback permission, call icon visibility.
- [Meta Analytics tab](#/analytics/meta-analytics-tab) — call analytics.
