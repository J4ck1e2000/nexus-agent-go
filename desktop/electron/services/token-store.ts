import fs from 'node:fs';
import path from 'node:path';

/**
 * Abstraction over Electron's safeStorage so the store can be unit tested
 * without a running Electron instance.
 */
export interface TokenCipher {
  isEncryptionAvailable(): boolean;
  encryptString(plainText: string): Buffer;
  decryptString(encrypted: Buffer): string;
}

const FLAG_ENCRYPTED = 1;
const FLAG_PLAINTEXT = 0;

/**
 * Persists the Gateway JWT inside userData. The token never travels to the
 * renderer: only the main process reads it to attach Authorization headers.
 *
 * File layout: [1 flag byte][payload]. Flag 1 = safeStorage-encrypted,
 * flag 0 = plaintext fallback for systems without an OS keyring.
 */
export class TokenStore {
  private readonly filePath: string;
  private readonly cipher: TokenCipher;

  constructor(options: { dir: string; cipher: TokenCipher }) {
    this.filePath = path.join(options.dir, 'nexus-token.bin');
    this.cipher = options.cipher;
  }

  save(token: string): void {
    const trimmed = token.trim();
    if (!trimmed) {
      this.clear();
      return;
    }

    let flag: number;
    let payload: Buffer;
    if (this.cipher.isEncryptionAvailable()) {
      payload = this.cipher.encryptString(trimmed);
      flag = FLAG_ENCRYPTED;
    } else {
      payload = Buffer.from(trimmed, 'utf8');
      flag = FLAG_PLAINTEXT;
    }

    const contents = Buffer.concat([Buffer.from([flag]), payload]);
    this.atomicWrite(contents);
  }

  load(): string | null {
    let contents: Buffer;
    try {
      contents = fs.readFileSync(this.filePath);
    } catch {
      return null;
    }
    if (contents.length < 1) return null;

    const flag = contents[0];
    const payload = contents.subarray(1);
    try {
      if (flag === FLAG_ENCRYPTED) {
        if (!this.cipher.isEncryptionAvailable()) return null;
        return this.cipher.decryptString(payload).trim() || null;
      }
      if (flag === FLAG_PLAINTEXT) {
        return payload.toString('utf8').trim() || null;
      }
      return null;
    } catch {
      // Corrupt or unreadable payload: drop it by treating as no token.
      return null;
    }
  }

  clear(): void {
    try {
      fs.rmSync(this.filePath, { force: true });
    } catch {
      // Best effort; a failed cleanup must not break logout.
    }
  }

  private atomicWrite(contents: Buffer): void {
    fs.mkdirSync(path.dirname(this.filePath), { recursive: true });
    const tmpPath = `${this.filePath}.tmp-${process.pid}`;
    fs.writeFileSync(tmpPath, contents, { mode: 0o600 });
    fs.renameSync(tmpPath, this.filePath);
  }
}
