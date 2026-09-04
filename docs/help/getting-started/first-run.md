# First run — org + admin + first message

Assumes MySQL / Redis / Kafka / HBase are already running (either native or containers — see [Overview](#/getting-started/overview)).

## 1. Clone + configure

```bash
git clone https://github.com/v-senthil/nudgeway.git
cd nudgeway
cp config/example.yaml config/local.yaml

# Generate a 32-byte KEK for envelope-encrypted integration credentials.
openssl rand -hex 32
# Paste the output into config/local.yaml → auth.credential_kek_hex.
```

## 2. Apply migrations

```bash
mysql -u root -proot -e \
  'CREATE DATABASE IF NOT EXISTS nudgeway CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;'

migrate -path migrations \
  -database 'mysql://root:root@tcp(127.0.0.1:3306)/nudgeway?parseTime=true&multiStatements=true' \
  up
```

## 3. Create an org + admin user

Build the CLI:

```bash
go build -o bin/nudgeway-cli ./cmd/cli
```

Then:

```bash
./bin/nudgeway-cli tenant create --slug acme --name "Acme Co"

./bin/nudgeway-cli user create \
  --org-slug acme --email you@acme.com --password password123 --admin
```

## 4. Start backend + frontend

```bash
make dev
```

Backend → `:8080`. Vite dev server → `:5173`. Ctrl-C stops both.

Open **http://localhost:5173** and log in.

## 5. Connect your first WhatsApp integration

Settings → Integrations → **Connect WhatsApp**. Paste the six fields:

| Field | Where from |
|---|---|
| Name | Any label ("Acme India") |
| Phone Number ID | Meta App → WhatsApp → API Setup |
| WABA ID | Same page |
| Access Token | System User token, `whatsapp_business_messaging` + `whatsapp_business_management` |
| App Secret | Meta App → Settings → Basic |
| Verify Token | Any string you pick; you'll paste the same value into Meta below |

Save. Nudgeway envelope-encrypts the secrets. See [Connect a WhatsApp integration](#/integrations/connect-whatsapp) for detail.

## 6. Point Meta at your webhook

The integration row shows its webhook URL (e.g. `https://<your-tunnel>.trycloudflare.com/webhooks/whatsapp/<integration_id>`). Two ways to get it into Meta:

- **From the UI**: click **Push to Meta** on the integration row's Details tab — auto-detects an ngrok tunnel and POSTs the `webhook_configuration` override for you.
- **Manually**: paste the URL into Meta App → WhatsApp → Configuration → Callback URL, paste your verify token, click **Verify and save**, then subscribe to `messages`, `message_status`, `calls`, and `call_settings_update`.

See [Push webhook to Meta](#/integrations/webhook-setup) for the full flow.

## 7. Send yourself a message

- **Inbound**: message the business number from your phone. The conversation appears live in the Inbox.
- **Outbound**: open the conversation, type in the composer, hit Send. Delivery ticks update in real time.

## Troubleshooting

- **Login fails** → the admin user wasn't created; re-run step 3 with `--admin` set.
- **Webhook verify returns 403** → the verify token you saved doesn't match what you're typing into Meta.
- **Outbound send returns "conversation not open"** → 24-hour customer-service window expired. Send an approved template ([Send a template message](#/inbox/send-template)) or wait for the customer to message you first.
- **Analytics cards all zero** → daily rollup runs every 15 minutes; restart the server for an immediate boot-tick.

See [Inbox troubleshooting](#/inbox/troubleshooting) and [Integrations](#/integrations/overview) for more.
