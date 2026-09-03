-- 20260903000002_phase1_domain.down.sql
-- Reverse Phase 1 domain in dependency order.

DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS conversations;
DROP TABLE IF EXISTS sessions_comm;
DROP TABLE IF EXISTS business_endpoints;
DROP TABLE IF EXISTS integration_credentials;
DROP TABLE IF EXISTS integrations;

-- contacts <-> contact_identities are mutually referential; drop the FK
-- on contacts.primary_identity_id first so both tables can be removed.
ALTER TABLE contacts DROP FOREIGN KEY fk_contacts_primary_identity;
DROP TABLE IF EXISTS contact_identities;
DROP TABLE IF EXISTS contacts;
