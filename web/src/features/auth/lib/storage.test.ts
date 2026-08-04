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
import { after, before, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import {
  clearResellerInvitation,
  getResellerInvitation,
  saveResellerInvitation,
} from './storage'

const originalWindow = globalThis.window

before(() => {
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: new Window({ url: 'https://dashboard.example.com' }),
  })
})

after(() => {
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: originalWindow,
  })
})

describe('reseller invitation storage', () => {
  test('keeps the opaque token for one tab and clears legacy affiliate state', () => {
    window.localStorage.setItem('aff', 'legacy-code')

    saveResellerInvitation('i1.opaque-token')

    assert.equal(getResellerInvitation(), 'i1.opaque-token')
    assert.equal(
      window.sessionStorage.getItem('reseller_invitation'),
      'i1.opaque-token'
    )
    assert.equal(window.localStorage.getItem('aff'), null)
  })

  test('clears the invitation after registration or OAuth completion', () => {
    saveResellerInvitation('i1.single-use-flow')
    clearResellerInvitation()

    assert.equal(getResellerInvitation(), '')
  })
})
