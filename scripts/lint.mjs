import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { join } from 'node:path';

const binName = process.platform === 'win32' ? 'eslint.cmd' : 'eslint';
const eslintBin = join(process.cwd(), 'node_modules', '.bin', binName);

if (!existsSync(eslintBin)) {
  console.warn('[lint] eslint is not installed locally. Skipping lint (warn-first mode).');
  process.exit(0);
}

const result = spawnSync(eslintBin, ['src/**/*.{ts,tsx}', '--max-warnings=9999'], {
  stdio: 'inherit',
  shell: false,
});

process.exit(result.status ?? 1);
