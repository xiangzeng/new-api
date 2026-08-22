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

import type { FlowQuotaDataItem } from '@/features/dashboard/types'

import {
  USER_USAGE_UNKNOWN_KEY,
  aggregateUserUsage,
  collectUserUsageOptions,
  filterUserUsageRows,
  getDefaultUserUsageRange,
  getUserUsageDimensionValue,
  getUserUsageMonthOptions,
  getUserUsageMonthRange,
  matchUserUsageMonth,
  sortUserUsageRows,
  type UserUsageTableRow,
} from '../user-usage'

const FLOW_ROWS: FlowQuotaDataItem[] = [
  {
    use_group: 'claude-kiro',
    model_name: 'claude-sonnet-4',
    channel_id: 7,
    channel_name: 'kiro-us',
    token_id: 21,
    token_name: 'cli',
    quota: 900,
    count: 30,
    token_used: 4500,
  },
  {
    use_group: 'claude-kiro',
    model_name: 'claude-haiku-4',
    channel_id: 7,
    channel_name: 'kiro-us',
    token_id: 21,
    token_name: 'cli',
    quota: 100,
    count: 20,
    token_used: 500,
  },
  {
    use_group: 'default',
    model_name: 'claude-sonnet-4',
    channel_id: 9,
    channel_name: '',
    token_id: 0,
    quota: 500,
    count: 10,
    token_used: 1000,
  },
]

describe('user usage dimension values', () => {
  test('uses the trimmed name as key for named dimensions', () => {
    assert.deepEqual(
      getUserUsageDimensionValue({ use_group: ' claude-kiro ' }, 'group'),
      { key: 'claude-kiro', name: 'claude-kiro' }
    )
  })

  test('collapses a missing group into the unknown bucket', () => {
    assert.deepEqual(getUserUsageDimensionValue({ use_group: '' }, 'group'), {
      key: USER_USAGE_UNKNOWN_KEY,
      name: '',
    })
  })

  test('falls back to the id when a channel has no name', () => {
    assert.deepEqual(
      getUserUsageDimensionValue(
        { channel_id: 9, channel_name: '' },
        'channel'
      ),
      { key: '9', name: '#9' }
    )
  })

  test('collapses a zero token id into the unknown bucket', () => {
    assert.deepEqual(
      getUserUsageDimensionValue({ token_id: 0, token_name: 'cli' }, 'token'),
      { key: USER_USAGE_UNKNOWN_KEY, name: '' }
    )
  })
})

describe('user usage row filtering', () => {
  test('returns every row when no filter is active', () => {
    assert.equal(filterUserUsageRows(FLOW_ROWS, {}).length, 3)
  })

  test('keeps only the rows of the selected group', () => {
    const filtered = filterUserUsageRows(FLOW_ROWS, { group: 'claude-kiro' })
    assert.equal(filtered.length, 2)
    assert.ok(filtered.every((row) => row.use_group === 'claude-kiro'))
  })

  test('combines filters from different dimensions with AND', () => {
    const filtered = filterUserUsageRows(FLOW_ROWS, {
      group: 'claude-kiro',
      model: 'claude-haiku-4',
    })
    assert.equal(filtered.length, 1)
    assert.equal(filtered[0].quota, 100)
  })

  test('returns an empty list for missing input', () => {
    assert.deepEqual(filterUserUsageRows(undefined, { group: 'x' }), [])
  })
})

describe('user usage aggregation', () => {
  test('sums quota, requests and tokens per group', () => {
    const summary = aggregateUserUsage(FLOW_ROWS, 'group')
    assert.equal(summary.totalQuota, 1500)
    assert.equal(summary.totalCount, 60)
    assert.equal(summary.totalTokens, 6000)
    assert.deepEqual(
      summary.rows.map((row) => [row.key, row.quota, row.count, row.tokens]),
      [
        ['claude-kiro', 1000, 50, 5000],
        ['default', 500, 10, 1000],
      ]
    )
  })

  test('ranks rows by consumption descending', () => {
    const summary = aggregateUserUsage(FLOW_ROWS, 'model')
    assert.deepEqual(
      summary.rows.map((row) => row.key),
      ['claude-sonnet-4', 'claude-haiku-4']
    )
  })

  test('reports each row share of the total quota', () => {
    const summary = aggregateUserUsage(FLOW_ROWS, 'group')
    assert.equal(summary.rows[0].share, 1000 / 1500)
    assert.equal(summary.rows[1].share, 500 / 1500)
  })

  test('returns zeroed totals for an empty range', () => {
    const summary = aggregateUserUsage([], 'group')
    assert.deepEqual(summary, {
      totalQuota: 0,
      totalCount: 0,
      totalTokens: 0,
      rows: [],
    })
  })

  test('keeps a zero-quota row without dividing by zero', () => {
    const summary = aggregateUserUsage(
      [{ use_group: 'free', quota: 0, count: 3, token_used: 0 }],
      'group'
    )
    assert.equal(summary.rows[0].share, 0)
    assert.equal(summary.rows[0].count, 3)
  })

  test('lists filter options ranked by consumption', () => {
    assert.deepEqual(collectUserUsageOptions(FLOW_ROWS, 'channel'), [
      { key: '7', name: 'kiro-us' },
      { key: '9', name: '#9' },
    ])
  })
})

describe('user usage row sorting', () => {
  const rows: UserUsageTableRow[] = [
    {
      key: 'b',
      name: 'b',
      label: 'b',
      quota: 10,
      count: 5,
      tokens: 1,
      share: 0.1,
    },
    {
      key: 'a',
      name: 'a',
      label: 'a',
      quota: 30,
      count: 1,
      tokens: 3,
      share: 0.3,
    },
  ]

  test('sorts by label ascending', () => {
    assert.deepEqual(
      sortUserUsageRows(rows, 'label', 'asc').map((row) => row.key),
      ['a', 'b']
    )
  })

  test('sorts by requests descending', () => {
    assert.deepEqual(
      sortUserUsageRows(rows, 'count', 'desc').map((row) => row.key),
      ['b', 'a']
    )
  })

  test('does not mutate the input array', () => {
    const input = [...rows]
    sortUserUsageRows(input, 'quota', 'asc')
    assert.deepEqual(
      input.map((row) => row.key),
      ['b', 'a']
    )
  })
})

describe('user usage time ranges', () => {
  const now = new Date(2026, 7, 22, 15, 30, 0)

  test('defaults to the last 30 days ending today', () => {
    const range = getDefaultUserUsageRange(now)
    assert.equal(range.start, new Date(2026, 6, 24, 0, 0, 0).getTime() / 1000)
    assert.equal(range.end, new Date(2026, 7, 22, 23, 59, 59).getTime() / 1000)
  })

  test('expands a month value into its calendar boundaries', () => {
    const range = getUserUsageMonthRange('2026-07')
    assert.deepEqual(range, {
      start: new Date(2026, 6, 1, 0, 0, 0).getTime() / 1000,
      end: new Date(2026, 6, 31, 23, 59, 59).getTime() / 1000,
    })
  })

  test('rejects a malformed month value', () => {
    assert.equal(getUserUsageMonthRange('2026-7'), null)
    assert.equal(getUserUsageMonthRange('not-a-month'), null)
  })

  test('lists recent months newest first across a year boundary', () => {
    assert.deepEqual(
      getUserUsageMonthOptions(3, new Date(2026, 1, 10, 0, 0, 0)),
      ['2026-02', '2026-01', '2025-12']
    )
  })

  test('recognises a range that covers exactly one month', () => {
    const range = getUserUsageMonthRange('2026-08')
    assert.ok(range)
    assert.equal(matchUserUsageMonth(range), '2026-08')
  })

  test('returns null for a range that is not a whole month', () => {
    assert.equal(matchUserUsageMonth(getDefaultUserUsageRange(now)), null)
  })
})
