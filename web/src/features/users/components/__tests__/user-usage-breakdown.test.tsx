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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'CustomEvent',
  'MouseEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { USER_USAGE_UNKNOWN_KEY } = await import('../../lib/user-usage')
const { UserUsageBreakdown } = await import('../detail/user-usage-breakdown')

type UsageSummary = Parameters<typeof UserUsageBreakdown>[0]['summary']

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        Group: 'Group',
        Model: 'Model',
        Channel: 'Channel',
        'Used Quota': 'Used Quota',
        Requests: 'Requests',
        Tokens: 'Tokens',
        Share: 'Share',
        Unknown: 'Unknown',
        'No usage data in the selected period':
          'No usage data in the selected period',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const SUMMARY: UsageSummary = {
  totalQuota: 1500,
  totalCount: 60,
  totalTokens: 6000,
  rows: [
    {
      key: 'claude-kiro',
      name: 'claude-kiro',
      quota: 1000,
      count: 10,
      tokens: 5000,
      share: 1000 / 1500,
    },
    {
      key: 'default',
      name: 'default',
      quota: 500,
      count: 50,
      tokens: 1000,
      share: 500 / 1500,
    },
  ],
}

const EMPTY_SUMMARY: UsageSummary = {
  totalQuota: 0,
  totalCount: 0,
  totalTokens: 0,
  rows: [],
}

function BreakdownHarness(props: {
  summary?: UsageSummary
  keyword?: string
  loading?: boolean
  onDimensionChange?: (
    dimension: 'group' | 'model' | 'channel' | 'token' | 'node'
  ) => void
}) {
  return (
    <I18nextProvider i18n={i18n}>
      <UserUsageBreakdown
        dimension='group'
        dimensions={['group', 'model', 'channel']}
        onDimensionChange={props.onDimensionChange ?? (() => {})}
        summary={props.summary ?? SUMMARY}
        keyword={props.keyword ?? ''}
        loading={props.loading ?? false}
      />
    </I18nextProvider>
  )
}

function rowLabels(container: HTMLElement): string[] {
  return [...container.querySelectorAll('tbody tr')].map(
    (row) => row.querySelector('td')?.textContent?.trim() ?? ''
  )
}

function headerButton(container: HTMLElement, label: string) {
  return [...container.querySelectorAll('thead button')].find((button) =>
    button.textContent?.trim().startsWith(label)
  ) as HTMLElement | undefined
}

describe('user usage breakdown table', () => {
  after(() => {
    domWindow.close()
  })

  test('ranks rows by consumed quota when no sort was chosen', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<BreakdownHarness />))

    assert.deepEqual(rowLabels(container), ['claude-kiro', 'default'])

    await act(async () => root.unmount())
    container.remove()
  })

  test('sorts by requests descending when the requests header is activated', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<BreakdownHarness />))

    const requestsHeader = headerButton(container, 'Requests')
    assert.ok(requestsHeader)
    await act(async () => {
      requestsHeader.click()
    })

    assert.deepEqual(rowLabels(container), ['default', 'claude-kiro'])
    assert.equal(requestsHeader.getAttribute('aria-pressed'), 'true')

    await act(async () => root.unmount())
    container.remove()
  })

  test('flips to ascending when the active sort header is activated again', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<BreakdownHarness />))

    const quotaHeader = headerButton(container, 'Used Quota')
    assert.ok(quotaHeader)
    await act(async () => {
      quotaHeader.click()
    })

    assert.deepEqual(rowLabels(container), ['default', 'claude-kiro'])

    await act(async () => root.unmount())
    container.remove()
  })

  test('keeps only the rows matching the keyword filter', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<BreakdownHarness keyword='KIRO' />))

    assert.deepEqual(rowLabels(container), ['claude-kiro'])

    await act(async () => root.unmount())
    container.remove()
  })

  test('shows the empty message when the range has no usage', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(<BreakdownHarness summary={EMPTY_SUMMARY} />)
    )

    assert.equal(
      container.textContent?.includes('No usage data in the selected period'),
      true
    )
    assert.deepEqual(rowLabels(container), [
      'No usage data in the selected period',
    ])

    await act(async () => root.unmount())
    container.remove()
  })

  test('replaces the table with a spinner while usage is loading', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<BreakdownHarness loading />))

    assert.equal(container.querySelector('table'), null)
    assert.ok(container.querySelector('.animate-spin'))

    await act(async () => root.unmount())
    container.remove()
  })

  test('labels a row without a dimension value as unknown', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () =>
      root.render(
        <BreakdownHarness
          summary={{
            totalQuota: 100,
            totalCount: 1,
            totalTokens: 10,
            rows: [
              {
                key: USER_USAGE_UNKNOWN_KEY,
                name: '',
                quota: 100,
                count: 1,
                tokens: 10,
                share: 1,
              },
            ],
          }}
        />
      )
    )

    assert.deepEqual(rowLabels(container), ['Unknown'])

    await act(async () => root.unmount())
    container.remove()
  })

  test('renders one tab per available breakdown dimension', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<BreakdownHarness />))

    const tabLabels = [...container.querySelectorAll('[role="tab"]')].map(
      (tab) => tab.textContent?.trim()
    )
    assert.deepEqual(tabLabels, ['Group', 'Model', 'Channel'])

    await act(async () => root.unmount())
    container.remove()
  })
})
