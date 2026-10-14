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
import { useMutation } from '@tanstack/react-query'
import { CheckCircle2, Loader2, XCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import dayjs from '@/lib/dayjs'

import { testBalanceKey } from '../api'
import {
  balanceDiagnoseCommand,
  balanceKeyFailureHint,
  formatOpenBalance,
} from '../lib/balance-key-test'
import type { BalanceKeyTestResult } from '../types'

type BalanceKeyTestPanelProps = {
  balanceKey: string
}

type BalanceKeyTestOutcomeProps = {
  result: BalanceKeyTestResult
  testedAt: number
}

function BalanceKeyTestOutcome(props: BalanceKeyTestOutcomeProps) {
  const { t } = useTranslation()
  const result = props.result
  const timing = t('{{elapsed}} ms · tested at {{time}}', {
    elapsed: result.elapsedMs,
    time: dayjs(props.testedAt).format('YYYY-MM-DD HH:mm:ss'),
  })

  if (result.kind === 'success') {
    return (
      <Alert>
        <CheckCircle2 aria-hidden='true' />
        <AlertTitle>{t('Balance query succeeded')}</AlertTitle>
        <AlertDescription>
          {t('Balance: {{balance}}', {
            balance: formatOpenBalance(result.balance),
          })}
          {' · '}
          {timing}
        </AlertDescription>
      </Alert>
    )
  }

  let statusLine = t('No HTTP response')
  let detail = result.message
  if (result.kind === 'http_error') {
    statusLine = [`HTTP ${result.status}`, result.code]
      .filter(Boolean)
      .join(' · ')
    detail = result.message || result.body
  }

  return (
    <Alert variant='destructive'>
      <XCircle aria-hidden='true' />
      <AlertTitle>{t('Balance query failed')}</AlertTitle>
      <AlertDescription className='space-y-1'>
        <p className='font-mono text-xs break-all'>{statusLine}</p>
        {detail ? (
          <p className='font-mono text-xs break-all'>{detail}</p>
        ) : null}
        <p>{balanceKeyFailureHint(result, t)}</p>
        <p className='text-xs'>{timing}</p>
      </AlertDescription>
    </Alert>
  )
}

/**
 * Runs a real balance query with a key and shows what came back, plus a shell
 * check for the server that will use the key. The browser test only proves
 * this computer can reach the site; a program on a server can still be stopped
 * by its own network or a firewall, which only the server check reveals.
 */
export function BalanceKeyTestPanel(props: BalanceKeyTestPanelProps) {
  const { t } = useTranslation()
  const testMutation = useMutation({
    mutationFn: () => testBalanceKey(props.balanceKey),
  })
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  const command = balanceDiagnoseCommand(props.balanceKey, origin)

  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-sm font-medium'>{t('Test this key')}</span>
        <Button
          type='button'
          variant='outline'
          size='sm'
          className='gap-2'
          disabled={testMutation.isPending}
          onClick={() => testMutation.mutate()}
        >
          {testMutation.isPending ? (
            <Loader2 className='h-4 w-4 animate-spin' aria-hidden='true' />
          ) : null}
          {t('Test')}
        </Button>
      </div>
      {testMutation.data ? (
        <BalanceKeyTestOutcome
          result={testMutation.data}
          testedAt={testMutation.submittedAt}
        />
      ) : null}
      <p className='text-muted-foreground text-xs'>
        {t(
          'This test runs from this browser, not from your server. If your program runs on a server, run the check below on that server and send the output to support.'
        )}
      </p>

      <div className='space-y-2 rounded-md border border-dashed p-3'>
        <div className='flex items-center justify-between gap-2'>
          <span className='text-sm font-medium'>
            {t('Check from your server')}
          </span>
          <CopyButton
            value={command}
            variant='ghost'
            className='size-8'
            iconClassName='size-4'
            tooltip={t('Copy command')}
            aria-label={t('Copy command')}
          />
        </div>
        <pre className='text-muted-foreground overflow-x-auto font-mono text-xs'>
          {command}
        </pre>
        <ul className='text-muted-foreground list-disc space-y-1 pl-4 text-xs'>
          <li>{t('HTTP 200: the server reaches us and the key works.')}</li>
          <li>
            {t(
              '"Connection timed out" or HTTP 000: the request never reached us. Send us the egress IP so we can check whether it is blocked.'
            )}
          </li>
          <li>
            {t('"Could not resolve host": DNS on that server is not working.')}
          </li>
          <li>
            {t(
              'HTTP 401, 403 or 429: the request reached us, and the response printed above the HTTP line says what is wrong.'
            )}
          </li>
        </ul>
      </div>
    </div>
  )
}
