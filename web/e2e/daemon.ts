import { spawn } from 'node:child_process'
import { existsSync, mkdtempSync, rmSync } from 'node:fs'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

export const BIN = resolve(import.meta.dirname, '../../bin/aim')

function freePort(): Promise<number> {
  return new Promise((res, rej) => {
    const s = createServer()
    s.once('error', rej)
    s.listen(0, '127.0.0.1', () => {
      const { port } = s.address() as { port: number }
      s.close(() => res(port))
    })
  })
}

export interface Daemon {
  /** Web console / REST base URL. */
  base: string
  /** Unix socket the `aim mcp` shim connects to. */
  socket: string
  stop: () => Promise<void>
}

/**
 * Starts bin/aim daemon on a temp db/socket and free loopback ports. The
 * binary must already exist (`task build`). Extra daemon flags go in `args`.
 */
export async function startDaemon(args: string[] = []): Promise<Daemon> {
  if (!existsSync(BIN)) throw new Error(`${BIN} not found; run "task build" first`)
  // /tmp keeps the unix socket path short.
  const dir = mkdtempSync(join(tmpdir(), 'aim-pw-'))
  const socket = join(dir, 'aim.sock')
  const [tcp, http] = [await freePort(), await freePort()]
  const proc = spawn(
    BIN,
    ['daemon', '--db', join(dir, 'aim.db'), '--socket', socket, '--listen', `127.0.0.1:${tcp}`, '--http', `127.0.0.1:${http}`, '--log-level', 'warn', ...args],
    { stdio: ['ignore', 'inherit', 'inherit'] },
  )
  let died = false
  proc.once('exit', () => (died = true))

  const base = `http://127.0.0.1:${http}`
  const deadline = Date.now() + 15_000
  for (;;) {
    if (died) throw new Error('aim daemon exited during startup')
    try {
      if ((await fetch(`${base}/healthz`)).ok) break
    } catch {
      /* not up yet */
    }
    if (Date.now() > deadline) {
      proc.kill('SIGTERM')
      throw new Error('aim daemon did not become healthy')
    }
    await new Promise((r) => setTimeout(r, 50))
  }

  return {
    base,
    socket,
    stop: async () => {
      if (!died) {
        proc.kill('SIGTERM')
        await new Promise<void>((res) => {
          const t = setTimeout(() => (proc.kill('SIGKILL'), res()), 5000)
          proc.once('exit', () => (clearTimeout(t), res()))
        })
      }
      rmSync(dir, { recursive: true, force: true })
    },
  }
}
