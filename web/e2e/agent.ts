import { spawn, type ChildProcessByStdio } from 'node:child_process'
import type { Readable, Writable } from 'node:stream'
import { createInterface } from 'node:readline'

interface Pending {
  resolve: (v: unknown) => void
  reject: (e: Error) => void
}

export interface ToolResult {
  isError: boolean
  /** structuredContent when the tool returned one, else the first text block. */
  data: any // eslint-disable-line @typescript-eslint/no-explicit-any
  text: string
}

/** A real `aim mcp` shim spoken to as an MCP client over newline JSON-RPC on stdio. */
export class Agent {
  private proc: ChildProcessByStdio<Writable, Readable, null>
  private nextId = 1
  private pending = new Map<number, Pending>()
  private exited: Promise<void>

  private constructor(
    bin: string,
    addr: string,
    readonly session: string,
    private readonly client: string,
  ) {
    this.proc = spawn(bin, ['mcp'], {
      env: { ...process.env, AIM_ADDR: addr, CLAUDE_CODE_SESSION_ID: session },
      stdio: ['pipe', 'pipe', 'inherit'],
    })
    this.exited = new Promise((res) => this.proc.once('exit', () => res()))
    createInterface({ input: this.proc.stdout }).on('line', (line) => {
      let msg: { id?: number; result?: unknown; error?: { message: string } }
      try {
        msg = JSON.parse(line)
      } catch {
        return
      }
      const p = msg.id === undefined ? undefined : this.pending.get(msg.id)
      if (!p || msg.id === undefined) return
      this.pending.delete(msg.id)
      if (msg.error) p.reject(new Error(msg.error.message))
      else p.resolve(msg.result)
    })
  }

  /** Spawns the shim and completes the MCP initialize handshake. */
  static async start(bin: string, addr: string, session: string, client: string): Promise<Agent> {
    const a = new Agent(bin, addr, session, client)
    await a.rpc('initialize', {
      protocolVersion: '2025-06-18',
      capabilities: {},
      clientInfo: { name: client, version: '1.0.0' },
    })
    a.notify('notifications/initialized')
    return a
  }

  private write(obj: unknown) {
    this.proc.stdin.write(JSON.stringify(obj) + '\n')
  }

  private notify(method: string) {
    this.write({ jsonrpc: '2.0', method })
  }

  private rpc(method: string, params: unknown): Promise<any> { // eslint-disable-line @typescript-eslint/no-explicit-any
    const id = this.nextId++
    return new Promise((resolve, reject) => {
      const t = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error(`${this.client}: ${method} timed out`))
      }, 15_000)
      this.pending.set(id, {
        resolve: (v) => (clearTimeout(t), resolve(v)),
        reject: (e) => (clearTimeout(t), reject(e)),
      })
      this.write({ jsonrpc: '2.0', id, method, params })
    })
  }

  /** Calls a tool; tool-level errors are returned (isError) rather than thrown. */
  async call(name: string, args: Record<string, unknown> = {}): Promise<ToolResult> {
    const res = await this.rpc('tools/call', { name, arguments: args })
    const text: string = res.content?.find((c: { type: string }) => c.type === 'text')?.text ?? ''
    return { isError: !!res.isError, data: res.structuredContent, text }
  }

  /** Like call, but throws on a tool error. */
  async must(name: string, args: Record<string, unknown> = {}): Promise<ToolResult> {
    const r = await this.call(name, args)
    if (r.isError) throw new Error(`${this.client} ${name}: ${r.text}`)
    return r
  }

  /** SIGTERM and wait; SIGKILL after 3s. */
  async stop() {
    if (this.proc.exitCode !== null || this.proc.signalCode !== null) return
    this.proc.stdin.end()
    this.proc.kill('SIGTERM')
    const t = setTimeout(() => this.proc.kill('SIGKILL'), 3000)
    await this.exited
    clearTimeout(t)
  }
}
