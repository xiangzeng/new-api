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
import { after, afterEach, describe, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window({ url: 'https://relay.example' })
const domGlobals = [
  'window',
  'document',
  'navigator',
  'localStorage',
  'sessionStorage',
  'HTMLElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'PointerEvent',
  'MouseEvent',
  'FocusEvent',
  'CustomEvent',
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
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { BalanceKeyTestPanel } = await import('../balance-key-test-panel')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { translation: {} } },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type FetchCall = { url: string; authorization: string | null }
type RenderedPanel = {
  host: HTMLDivElement
  root: ReturnType<typeof createRoot>
}

const originalFetch = globalThis.fetch
let rendered: RenderedPanel | null = null

function installFetch(status: number, body: unknown, calls: FetchCall[]) {
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({
      url: String(input),
      authorization: new Headers(init?.headers).get('Authorization'),
    })
    return new Response(JSON.stringify(body), { status })
  }) as typeof fetch
}

async function renderPanel(balanceKey: string): Promise<RenderedPanel> {
  const host = document.createElement('div')
  document.body.append(host)
  const root = createRoot(host)
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  await act(async () => {
    root.render(
      <QueryClientProvider client={queryClient}>
        <I18nextProvider i18n={i18n}>
          <BalanceKeyTestPanel balanceKey={balanceKey} />
        </I18nextProvider>
      </QueryClientProvider>
    )
  })
  rendered = { host, root }
  return rendered
}

function testButton(host: HTMLElement): HTMLButtonElement {
  const button = [...host.querySelectorAll('button')].find(
    (candidate) => candidate.textContent?.trim() === 'Test'
  )
  assert.ok(button, 'Test button should be rendered')
  return button
}

async function clickTestAndWaitForResult(host: HTMLElement) {
  await act(async () => {
    testButton(host).click()
  })
  // The mocked fetch resolves immediately, but the mutation settles over a few
  // task turns; yield until the result is on screen rather than sleeping.
  for (let attempt = 0; attempt < 50; attempt += 1) {
    if (host.querySelector('[role="alert"]')) return
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0))
    })
  }
  assert.fail('a test result should be shown')
}

describe('balance key test panel', () => {
  afterEach(async () => {
    globalThis.fetch = originalFetch
    if (rendered) {
      const current = rendered
      await act(async () => current.root.unmount())
      current.host.remove()
      rendered = null
    }
  })

  after(() => {
    domWindow.close()
  })

  test('a rejected key shows the open API error code and the fix', async () => {
    const calls: FetchCall[] = []
    installFetch(
      401,
      {
        success: false,
        code: 'CREDENTIAL_REVOKED',
        message: 'The credential has been revoked. Issue a new balance key.',
      },
      calls
    )
    const panel = await renderPanel('obk_revoked')

    await clickTestAndWaitForResult(panel.host)

    const alert = panel.host.querySelector('[role="alert"]')
    assert.match(alert?.textContent ?? '', /HTTP 401 · CREDENTIAL_REVOKED/)
    assert.match(alert?.textContent ?? '', /Create a new one/)
    assert.deepEqual(calls, [
      { url: '/api/open/v1/balance', authorization: 'Bearer obk_revoked' },
    ])
  })

  test('a working key shows the balance in the site currency', async () => {
    installFetch(
      200,
      {
        success: true,
        data: {
          quota: 5_000_000,
          used_quota: 0,
          balance: 10,
          used: 0,
          display_type: 'USD',
          currency_symbol: '$',
        },
      },
      []
    )
    const panel = await renderPanel('obk_valid')

    await clickTestAndWaitForResult(panel.host)

    const alert = panel.host.querySelector('[role="alert"]')
    assert.match(alert?.textContent ?? '', /Balance query succeeded/)
    assert.match(alert?.textContent ?? '', /\$10\.00/)
  })

  test('the server check is offered with the key before any test runs', async () => {
    const panel = await renderPanel('obk_valid')

    const command = panel.host.querySelector('pre')?.textContent ?? ''

    assert.match(command, /Bearer obk_valid/)
    assert.match(command, /https:\/\/relay\.example\/api\/open\/v1\/balance/)
    assert.equal(panel.host.querySelector('[role="alert"]'), null)
  })
})
