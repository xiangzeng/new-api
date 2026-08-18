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

import {
  buildRecentUsageDateRange,
  mergeRecentUsageDays,
} from '../channel-recent-usage'

describe('channel recent usage helpers', () => {
  test('builds a three-day local date range ending at the anchor day', () => {
    const range = buildRecentUsageDateRange(new Date(2026, 6, 8, 9, 30), 3)

    assert.equal(range.startDate, '2026-07-06')
    assert.equal(range.endDate, '2026-07-08')
    assert.deepEqual(
      range.days.map((day) => [day.date, day.labelKey]),
      [
        ['2026-07-08', 'Today'],
        ['2026-07-07', 'Yesterday'],
        ['2026-07-06', 'Day before yesterday'],
      ]
    )
  })

  test('merges sparse API rows into all visible days with zero defaults', () => {
    const range = buildRecentUsageDateRange(new Date(2026, 6, 8), 3)
    const rows = mergeRecentUsageDays(range.days, [
      {
        date: '2026-07-08',
        quota_used: 120,
        request_count: 2,
        token_used: 300,
      },
      {
        date: '2026-07-06',
        quota_used: 40,
        request_count: 1,
        token_used: 100,
      },
    ])

    assert.deepEqual(
      rows.map((row) => ({
        date: row.date,
        labelKey: row.labelKey,
        quota_used: row.quota_used,
        request_count: row.request_count,
        token_used: row.token_used,
      })),
      [
        {
          date: '2026-07-08',
          labelKey: 'Today',
          quota_used: 120,
          request_count: 2,
          token_used: 300,
        },
        {
          date: '2026-07-07',
          labelKey: 'Yesterday',
          quota_used: 0,
          request_count: 0,
          token_used: 0,
        },
        {
          date: '2026-07-06',
          labelKey: 'Day before yesterday',
          quota_used: 40,
          request_count: 1,
          token_used: 100,
        },
      ]
    )
  })
})
