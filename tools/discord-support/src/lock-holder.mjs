// Executed only by /usr/bin/lockf. The stdin pipe belongs to the owning service;
// EOF (including owner SIGKILL) releases the kernel lock without PID guessing.
process.stdout.write('LOCKED\n');
process.stdin.resume();
process.stdin.once('end',() => process.exit(0));
process.once('SIGTERM',() => process.exit(0));
process.once('SIGINT',() => process.exit(0));
