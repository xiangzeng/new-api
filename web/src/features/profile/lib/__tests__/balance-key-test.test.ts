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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { TFunction } from 'i18next'

import {
  balanceDiagnoseCommand,
  balanceKeyFailureHint,
  balanceKeyMatchesHint,
  formatOpenBalance,
  interpretBalanceResponse,
} from '../balance-key-test'

const translate = ((key: string) => key) as TFunction

const balancePayload = {
  success: true,
  data: {
    quota: 5_000_000,
    used_quota: 250_000,
    balance: 10,
    used: 0.5,
    display_type: 'USD',
    currency_symbol: '$',
  },
}

describe('interpreting a balance key test response', () => {
  test('a 200 with balance data is a success carrying the balance', () => {
    const result = interpretBalanceResponse(
      200,
      JSON.stringify(balancePayload),
      42
    )

    assert.equal(result.kind, 'success')
    assert.equal(result.kind === 'success' && result.balance.balance, 10)
    assert.equal(result.elapsedMs, 42)
  })

  test('an open API 401 keeps the error code and message verbatim', () => {
    const result = interpretBalanceResponse(
      401,
      JSON.stringify({
        success: false,
        code: 'CREDENTIAL_INVALID',
        message: 'Missing bearer credential.',
      }),
      15
    )

    assert.deepEqual(
      result.kind === 'http_error' && {
        status: result.status,
        code: result.code,
        message: result.message,
      },
      {
        status: 401,
        code: 'CREDENTIAL_INVALID',
        message: 'Missing bearer credential.',
      }
    )
  })

  test('a non-JSON proxy error page is a failure with a truncated body', () => {
    const page = `<html>${'x'.repeat(2000)}</html>`

    const result = interpretBalanceResponse(502, page, 30)

    assert.equal(result.kind, 'http_error')
    assert.equal(result.kind === 'http_error' && result.code, '')
    assert.equal(result.kind === 'http_error' && result.body.length, 500)
  })
})

describe('explaining a failed balance key test', () => {
  test('each open API error code gets its own explanation', () => {
    const hints = ['CREDENTIAL_INVALID', 'CREDENTIAL_REVOKED', 'RATE_LIMITED'].map(
      (code) =>
        balanceKeyFailureHint(
          {
            kind: 'http_error',
            status: code === 'RATE_LIMITED' ? 429 : 401,
            elapsedMs: 1,
            code,
            message: '',
            body: '',
          },
          translate
        )
    )

    assert.equal(new Set(hints).size, 3)
    assert.match(hints[0], /Authorization: Bearer/)
  })

  test('a server error without an open API code points at the status', () => {
    const hint = balanceKeyFailureHint(
      {
        kind: 'http_error',
        status: 502,
        elapsedMs: 1,
        code: '',
        message: '',
        body: '',
      },
      translate
    )

    assert.match(hint, /server failed/)
  })

  test('no HTTP answer at all points at this computer and the server check', () => {
    const hint = balanceKeyFailureHint(
      { kind: 'network_error', elapsedMs: 15_000, message: 'Failed to fetch' },
      translate
    )

    assert.match(hint, /server check below/)
  })
})

describe('matching a pasted key to its list entry', () => {
  test('a key with the hinted prefix and tail matches', () => {
    assert.equal(balanceKeyMatchesHint('obk_9f8e7d6c5b4a1b2c3', 'obk_…a1b2c3'), true)
  })

  test('a key with a different tail does not match', () => {
    assert.equal(balanceKeyMatchesHint('obk_9f8e7d6c5b4zzzzzz', 'obk_…a1b2c3'), false)
  })

  test('an API token with the same tail but another prefix does not match', () => {
    assert.equal(balanceKeyMatchesHint('sk-9f8e7d6c5b4a1b2c3', 'obk_…a1b2c3'), false)
  })

  test('the hint text itself is not accepted as a key', () => {
    assert.equal(balanceKeyMatchesHint('obk_a1b2c3', 'obk_…a1b2c3'), false)
  })
})

describe('showing the balance and the server check', () => {
  test('a currency balance shows the symbol and two decimals', () => {
    assert.equal(formatOpenBalance(balancePayload.data), '$10.00')
  })

  test('a token display shows the whole number of tokens', () => {
    assert.equal(
      formatOpenBalance({
        ...balancePayload.data,
        display_type: 'TOKENS',
        currency_symbol: '',
        balance: 5_000_000,
      }),
      '5000000'
    )
  })

  test('the server check calls this site with the key and reports the egress IP', () => {
    const command = balanceDiagnoseCommand(
      'obk_secret',
      'https://www.relayaicheap.com'
    )

    assert.match(command, /-H "Authorization: Bearer obk_secret"/)
    assert.match(
      command,
      /https:\/\/www\.relayaicheap\.com\/api\/open\/v1\/balance/
    )
    assert.match(command, /--connect-timeout 10/)
    assert.match(command, /%\{time_connect\}/)
    assert.match(command, /Egress IP/)
  })
})
