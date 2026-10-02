import { DatabaseSync } from 'node:sqlite';
import { randomUUID } from 'node:crypto';
import { join } from 'node:path';
import { privateDirectory, privateFile } from './private.mjs';
import { fail } from './errors.mjs';
export class Store {
  constructor(directory) {
    process.umask(0o077);
    privateDirectory(directory);
    const path = join(directory, 'tickets.sqlite');
    privateFile(path);
    this.db = new DatabaseSync(path);
    // DELETE journal avoids persistent WAL sidecars. Every process sets umask 0077.
    this.db.exec('PRAGMA journal_mode=DELETE; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON; PRAGMA secure_delete=ON;');
    const version = this.db.prepare('PRAGMA user_version').get().user_version;
    if (version > 1) fail('SCHEMA_TOO_NEW');
    this.db.exec(`BEGIN IMMEDIATE;
      CREATE TABLE IF NOT EXISTS tickets (
        id TEXT PRIMARY KEY, request_key TEXT UNIQUE NOT NULL,
        guild_id TEXT NOT NULL, requester_id TEXT NOT NULL,
        subject TEXT, description TEXT,
        channel_id TEXT UNIQUE, message_id TEXT,
        state TEXT NOT NULL CHECK(state IN ('creating','open','closing','closed')),
        create_attempted INTEGER NOT NULL DEFAULT 0,
        message_attempted INTEGER NOT NULL DEFAULT 0,
        claimed_by TEXT, created_at TEXT NOT NULL
      ) STRICT;
      CREATE TABLE IF NOT EXISTS secrets (key TEXT PRIMARY KEY, value TEXT NOT NULL) STRICT;
      CREATE TABLE IF NOT EXISTS resources (key TEXT PRIMARY KEY, id TEXT, attempted INTEGER NOT NULL DEFAULT 0) STRICT;
      CREATE TABLE IF NOT EXISTS audit (ticket_id TEXT NOT NULL, action TEXT NOT NULL, at TEXT NOT NULL) STRICT;
      PRAGMA user_version=1; COMMIT;`);
  }
  reserve({ requestKey, guildId, requesterId, subject, description }) {
    this.db.prepare(`INSERT OR IGNORE INTO tickets
      (id,request_key,guild_id,requester_id,subject,description,state,created_at)
      VALUES (?,?,?,?,?,?,'creating',?)`).run(randomUUID(), requestKey, guildId, requesterId, subject, description, new Date().toISOString());
    const t = this.db.prepare('SELECT * FROM tickets WHERE request_key=?').get(requestKey);
    if (!t || t.guild_id !== guildId || t.requester_id !== requesterId) fail('REQUEST_BINDING_MISMATCH');
    return t;
  }
  signingKey() {
    this.db.prepare("INSERT OR IGNORE INTO secrets VALUES ('signing-key',?)").run(randomUUID()+randomUUID());
    return this.db.prepare("SELECT value FROM secrets WHERE key='signing-key'").get().value;
  }
  ticket(id) { const t = this.db.prepare('SELECT * FROM tickets WHERE id=?').get(id); if (!t) fail('TICKET_UNKNOWN'); return t; }
  all() { return this.db.prepare('SELECT * FROM tickets ORDER BY created_at,id').all(); }
  update(id, changes) {
    const allowed = ['channel_id','message_id','state','create_attempted','message_attempted','subject','description'];
    if (!Object.keys(changes).length || Object.keys(changes).some(k => !allowed.includes(k))) fail('STORE_UPDATE_INVALID');
    this.db.prepare(`UPDATE tickets SET ${Object.keys(changes).map(k => `${k}=?`).join(',')} WHERE id=?`).run(...Object.values(changes), id);
    return this.ticket(id);
  }
  claim(id, actorId) {
    this.db.prepare("UPDATE tickets SET claimed_by=? WHERE id=? AND state='open' AND claimed_by IS NULL").run(actorId, id);
    const t = this.ticket(id);
    if (t.claimed_by !== actorId) fail('TICKET_ALREADY_CLAIMED');
    return t;
  }
  audit(id, action) {
    if (!['opened','claimed','closing','closed'].includes(action)) fail('AUDIT_ACTION_INVALID');
    this.db.prepare('INSERT INTO audit VALUES (?,?,?)').run(id, action, new Date().toISOString());
  }
  resource(key) {
    this.db.prepare('INSERT OR IGNORE INTO resources(key) VALUES (?)').run(key);
    return this.db.prepare('SELECT * FROM resources WHERE key=?').get(key);
  }
  attemptResource(key) { this.resource(key); this.db.prepare('UPDATE resources SET attempted=1 WHERE key=?').run(key); }
  bindResource(key, id) { this.resource(key); this.db.prepare('UPDATE resources SET id=? WHERE key=?').run(id, key); }
  close() { this.db.close(); }
}
