# Roadmap — what's coming next

This page tracks features that are on the near-term roadmap. Everything listed here is planned but **not yet shipped** — you can't use it today. When a feature ships, it moves out of this page and into its own help section.

Watch the [GitHub repo](https://github.com/v-senthil/nudgeway) for release notes.

## Broadcast campaigns

Send an approved template message to a curated audience — think order-shipped nudges, appointment reminders, promotional pushes.

- Audience builder over your contacts (tags, custom fields, saved segments)
- Template picker + parameter binding from contact fields
- Scheduled send (immediate or future date-time)
- Per-org rate-limit that respects Meta's messaging tiers
- Pause / resume / cancel a live campaign
- Per-recipient status: queued / sent / delivered / read / failed
- Retries + per-message idempotency

## Click-to-WhatsApp Ads (CTWA)

When a customer clicks your Facebook or Instagram ad and lands in WhatsApp, capture the ad payload and thread it through the conversation.

- Automatic capture of the ad referral (source URL, headline, media type) on the first inbound message
- Attribution visible on the contact profile + the conversation header
- Filterable in the inbox: "show me leads from ad campaign X"
- Analytics: cost-per-conversation broken down by ad

## WhatsApp Flows

Publish interactive forms customers fill inside WhatsApp — checkout, appointment booking, lead capture, feedback.

- Import Flow JSON built in Meta's Flow Builder
- Send a Flow as a message from a conversation or a template
- Responses render inline in the inbox thread as structured data (not just a JSON blob)
- Custom endpoint for dynamic Flows (data screens fetched from your backend)

## Conversational AI bots

First-touch AI agent that answers common questions, then hands off to a human when confidence drops.

- Anthropic (Claude) adapter first
- OpenAI adapter next
- Google AI + Zoho Zia later
- Human-handoff state machine: AI_ACTIVE ↔ HUMAN_ACTIVE ↔ AI_PAUSED, preserves full conversation context
- Agents see everything the bot said + the confidence trace that triggered handoff
- Configurable per integration or per conversation

## Third-party bot providers

Bring your existing bot to Nudgeway without rewriting it.

- Dialogflow adapter
- Azure Bot Service adapter
- Your bot handles the reply; Nudgeway remains the operator-facing inbox
- Same human-handoff semantics as the built-in AI

## Ticketing adapters

Turn conversations into support tickets in your existing help desk.

- Zoho Desk first (highest customer demand)
- Freshdesk + Zendesk to follow
- Auto-open a ticket when a conversation is created (or on a rule)
- Two-way sync: ticket status + notes flow both directions
- Ticket link visible on the conversation header

## Automation engine

If-this-then-that rules that run against every inbound event.

- Triggers: message received, tag added, conversation opened, call ended, etc.
- Conditions: contact tag, message contains, custom field, integration, business hours
- Actions: send message, add tag, assign team, open ticket, invoke AI, delay, webhook
- Runs against the canonical event bus so it works across every provider

## Additional channels

Same canonical inbox, more places to reach customers.

- Telegram
- Instagram Direct
- Facebook Messenger
- Each ships as an adapter, so the domain model, RBAC, audit, and MCP surface pick them up automatically

## Plugin marketplace

Community-authored provider adapters and automation actions, discoverable from Settings.

- Ships after the automation engine and a stable plugin loader
- Sandboxed execution, per-org install + config
- Marketplace listing curated in the repo initially, first-party listing site later

## When?

We ship in the open. There's no fixed date on any of these — priority moves based on user pull. Vote or comment on the [GitHub issues](https://github.com/v-senthil/nudgeway/issues) tagged `roadmap` to weight what lands first.
