// Starts a fresh register_manager server for end-to-end tests: builds the
// binary (with the already built frontend), sets the super admin's password
// through the CLI, then serves on 127.0.0.1:18080 with a throwaway database.
import { execFileSync, spawn } from 'node:child_process'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const repo = join(here, '../..')
const tmp = join(here, '.tmp')
rmSync(tmp, { recursive: true, force: true })
mkdirSync(tmp, { recursive: true })
const bin = join(tmp, process.platform === 'win32' ? 'server.exe' : 'server')

const env = {
  ...process.env,
  CGO_ENABLED: '0',
  RM_DB_PATH: join(tmp, 'e2e.db'),
  RM_LISTEN: '127.0.0.1:18080',
  RM_COOKIE_SECURE: 'false',
  RM_LOG_LEVEL: 'warn',
}

execFileSync('go', ['build', '-o', bin, './cmd/server'], { cwd: repo, env, stdio: 'inherit' })
// Set the built-in super admin's password (login "admin") before the first
// start, as a deployer can.
execFileSync(bin, ['admin', 'reset-password'], {
  env,
  input: 'root-password\n',
  stdio: ['pipe', 'inherit', 'inherit'],
})
const server = spawn(bin, ['serve'], { env, stdio: 'inherit' })
const stop = () => server.kill()
process.on('SIGINT', stop)
process.on('SIGTERM', stop)
process.on('exit', stop)
server.on('exit', (code) => process.exit(code ?? 0))
