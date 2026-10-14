/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type { TFunction } from 'i18next'

import type { BalanceKeyTestResult, OpenBalance } from '../types'

export const OPEN_BALANCE_PATH = '/api/open/v1/balance'

// Long error pages (an HTML 502 from a proxy, say) are cut so the result stays
// readable; the status line and code already say what kind of failure it was.
const MAX_BODY_PREVIEW = 500

type OpenApiPayload = {
  success?: boolean
  code?: string
  message?: string
  data?: OpenBalance
}

function parsePayload(body: string): OpenApiPayload | null {
  try {
    const parsed: unknown = JSON.parse(body)
    return parsed !== null && typeof parsed === 'object'
      ? (parsed as OpenApiPayload)
      : null
  } catch {
    return null
  }
}

/**
 * Turns a raw HTTP answer from the balance endpoint into a test result. Any
 * status other than a successful 200 is a failure, and the open API's own error
 * code and message are kept verbatim because they are what support searches for.
 */
export function interpretBalanceResponse(
  status: number,
  body: string,
  elapsedMs: number
): BalanceKeyTestResult {
  const payload = parsePayload(body)
  if (status === 200 && payload?.success && payload.data) {
    return { kind: 'success', status, elapsedMs, balance: payload.data }
  }
  return {
    kind: 'http_error',
    status,
    elapsedMs,
    code: payload?.code ?? '',
    message: payload?.message ?? '',
    body: body.slice(0, MAX_BODY_PREVIEW),
  }
}

/**
 * What a failed test means and what to do next. The open API error codes map
 * one to one; anything else falls back on the HTTP status.
 */
export function balanceKeyFailureHint(
  result: BalanceKeyTestResult,
  t: TFunction
): string {
  if (result.kind === 'success') return ''
  if (result.kind === 'network_error') {
    return t(
      'This browser could not reach the server. Check the network or proxy on this computer, then run the server check below.'
    )
  }
  switch (result.code) {
    case 'CREDENTIAL_INVALID':
      return t(
        'The key is missing, incomplete or wrong. Send the whole key as "Authorization: Bearer <key>".'
      )
    case 'CREDENTIAL_REVOKED':
      return t('This key has been revoked. Create a new one.')
    case 'USER_DISABLED':
      return t('This account is disabled.')
    case 'RATE_LIMITED':
      return t(
        'Too many balance queries in a short time. Wait a minute and try again.'
      )
  }
  if (result.status >= 500) {
    return t(
      'The server failed to handle the request. Send the test time and this result to support.'
    )
  }
  return t(
    'Unexpected response. Send the test time and this result to support.'
  )
}

/**
 * Whether a pasted key is the one a list entry stands for. The list only keeps
 * a hint such as "obk_…a1b2c3", so the prefix and the visible tail must match.
 */
export function balanceKeyMatchesHint(key: string, hint: string): boolean {
  const separator = hint.indexOf('…')
  if (separator < 0) return key === hint
  const prefix = hint.slice(0, separator)
  const suffix = hint.slice(separator + 1)
  return (
    key.length > prefix.length + suffix.length &&
    key.startsWith(prefix) &&
    key.endsWith(suffix)
  )
}

/** Balance shown in the site's display unit, matching the dashboard. */
export function formatOpenBalance(balance: OpenBalance): string {
  if (balance.display_type === 'TOKENS') {
    return String(Math.round(balance.balance))
  }
  return `${balance.currency_symbol}${balance.balance.toFixed(2)}`
}

/**
 * A shell check to run on the machine that actually calls the API. A browser
 * test only proves this computer can reach the site; this one shows where a
 * server's request stops (DNS, TCP, TLS or HTTP) and which egress IP it uses,
 * which is what a firewall or allow-list check needs.
 */
export function balanceDiagnoseCommand(key: string, origin: string): string {
  return [
    'curl -sS --connect-timeout 10 --max-time 20 \\',
    `  -H "Authorization: Bearer ${key}" \\`,
    "  -w '\\nHTTP %{http_code} | IP %{remote_ip} | DNS %{time_namelookup}s | TCP %{time_connect}s | TLS %{time_appconnect}s | Total %{time_total}s\\n' \\",
    `  ${origin}${OPEN_BALANCE_PATH}`,
    'echo "Egress IP: $(curl -sS --max-time 10 https://api.ipify.org)"',
  ].join('\n')
}
