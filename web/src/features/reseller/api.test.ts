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

import { normalizeResellerPage, parseTransferRecipient } from './api'

describe('reseller page normalization', () => {
  test('preserves collection responses', () => {
    const items = [{ id: 1 }]

    assert.deepEqual(
      normalizeResellerPage({ page: 1, page_size: 50, total: 1, items }),
      { page: 1, page_size: 50, total: 1, items }
    )
  })

  test('normalizes legacy null collections to an empty array', () => {
    assert.deepEqual(
      normalizeResellerPage({ page: 1, page_size: 50, total: 0, items: null }),
      { page: 1, page_size: 50, total: 0, items: [] }
    )
  })
})

describe('reseller transfer recipient parsing', () => {
  test('recognizes usernames and 32-character receive codes', () => {
    assert.deepEqual(parseTransferRecipient('recipient-user'), {
      recipient_username: 'recipient-user',
    })
    assert.deepEqual(
      parseTransferRecipient('AbCdEfGhIjKlMnOpQrStUvWxYz123456'),
      { recipient_public_id: 'AbCdEfGhIjKlMnOpQrStUvWxYz123456' }
    )
  })

  test('extracts a receive code from a receive link', () => {
    assert.deepEqual(
      parseTransferRecipient(
        'https://example.test/reseller?receive=0123456789abcdefghijklmnopqrstuv'
      ),
      { recipient_public_id: '0123456789abcdefghijklmnopqrstuv' }
    )
  })
})
