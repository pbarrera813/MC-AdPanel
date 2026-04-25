import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { join } from 'node:path';

const tscJs = join(process.cwd(), 'node_modules', 'typescript', 'bin', 'tsc');

if (!existsSync(tscJs)) {
  console.warn('[typecheck] typescript is not installed locally. Skipping typecheck (warn-first mode).');
  process.exit(0);
}

const nodeBin = process.execPath;
const result = spawnSync(nodeBin, [tscJs, '--noEmit', '--pretty'], {
  stdio: 'inherit',
  shell: false,
});

process.exit(result.status ?? 1);
