-- PostgreSQL integration sketch. Parameters marked :name are application
-- parameters, not literal psql syntax. Run crypto verification BEFORE this
-- transaction; never grant a session before COMMIT succeeds.
--
-- Required schema invariants:
--   credentials.id PRIMARY KEY (global across all accounts)
--   pending.flow_id PRIMARY KEY; pending stores trusted State and browser binding
--   accounts.generation incremented on recovery/security invalidation
--   credentials.version incremented on any credential update/replacement
--   sessions.token_hash UNIQUE (store a hash of the random session token)
--
-- All credential revoke/recovery writers use the same lock order:
-- account -> credential -> pending. Registration also locks the account first.
-- Absent account, key, or pending row is a failure, never an implicit creation.

BEGIN;
SELECT generation FROM accounts WHERE id = :verified_user_id FOR UPDATE;
-- Require generation = :generation_read_before_verify.
SELECT version, user_id, revoked FROM credentials
 WHERE id = :verified_credential_id FOR UPDATE;
-- Require version = :version_read_before_verify, same owner and not revoked.
SELECT state, browser_binding FROM pending WHERE flow_id = :flow_id FOR UPDATE;
-- Require same pending operation/challenge and protected browser binding.
-- Recheck expires_at against clock_timestamp(), not transaction start time.
-- Apply counter-anomaly policy before any writes (the demo rejects anomalies).
DELETE FROM pending
 WHERE flow_id = :flow_id AND expires_at > clock_timestamp()
 RETURNING flow_id;
-- Require exactly one returned row, otherwise ROLLBACK.
UPDATE credentials SET sign_count = :verified_count,
 backup_state = :verified_backup_state, version = version + 1
 WHERE id = :verified_credential_id;
INSERT INTO sessions(token_hash, user_id, account_generation)
 VALUES (:session_token_hash, :verified_user_id, :generation_read_before_verify);
COMMIT;
-- Only now return the session cookie. On any SQL/commit error: no cookie.
-- If COMMIT succeeds and the HTTP response is lost, the flow stays consumed.
-- Do not retry by issuing another session from the same proof.
--
-- Registration uses the same transaction boundary: recheck account generation
-- and fresh authorization; INSERT credential (global unique ID); DELETE pending;
-- create session if authorized; COMMIT. A duplicate credential rolls back all
-- writes. A new-account flow creates the account in this transaction as well.
