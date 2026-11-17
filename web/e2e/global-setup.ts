import { BIN, startDaemon } from './daemon'

/**
 * Starts the shared bin/aim daemon and publishes its addresses to the
 * workers through AIM_E2E_* env vars. The binary must already exist
 * (`task build`).
 */
export default async function globalSetup() {
  const d = await startDaemon()
  process.env.AIM_E2E_BASE_URL = d.base
  process.env.AIM_E2E_SOCKET = d.socket
  process.env.AIM_E2E_BIN = BIN
  return d.stop
}
