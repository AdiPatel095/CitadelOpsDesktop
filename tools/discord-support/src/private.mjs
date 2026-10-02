import { constants as F, closeSync, openSync, fstatSync, readFileSync, mkdirSync, lstatSync, writeFileSync, renameSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { dirname } from 'node:path';
import { fail } from './errors.mjs';
export function privateDirectory(path) {
  mkdirSync(path, { recursive: true, mode: 0o700 });
  const s = lstatSync(path);
  if (!s.isDirectory() || s.isSymbolicLink() || s.uid !== process.getuid() || (s.mode & 0o777) !== 0o700) fail('PRIVATE_DIRECTORY_REQUIRED');
}
export function privateRead(path) {
  let fd;
  try {
    fd = openSync(path, F.O_RDONLY | F.O_NOFOLLOW | F.O_NONBLOCK);
    const s = fstatSync(fd);
    if (!s.isFile() || s.uid !== process.getuid() || (s.mode & 0o077) || s.nlink !== 1) fail('PRIVATE_FILE_REQUIRED');
    return readFileSync(fd, 'utf8');
  } finally { if (fd !== undefined) closeSync(fd); }
}
export function privateFile(path) {
  const fd = openSync(path, F.O_CREAT | F.O_RDWR | F.O_NOFOLLOW, 0o600);
  try {
    const s = fstatSync(fd);
    if (!s.isFile() || s.uid !== process.getuid() || (s.mode & 0o777) !== 0o600 || s.nlink !== 1) fail('PRIVATE_FILE_REQUIRED');
  } finally { closeSync(fd); }
}
export function atomicPrivateJSON(path, data) {
  privateDirectory(dirname(path));
  const tmp = `${path}.${randomUUID()}.tmp`;
  writeFileSync(tmp, JSON.stringify(data), { mode: 0o600, flag: 'wx' });
  renameSync(tmp, path);
}
