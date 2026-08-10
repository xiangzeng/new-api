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
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { getSystemOptions } from '@/features/system-settings/api'
import { useUpdateOption } from '@/features/system-settings/hooks/use-update-option'

/** Option keys registered by `setting/system_setting/open_balance_api.go`. */
const OPTION_KEYS = {
  enabled: 'open_balance_api.enabled',
  exchangePerMinute: 'open_balance_api.exchange_rate_limit_per_minute',
  exchangeIpPerMinute: 'open_balance_api.exchange_ip_rate_limit_per_minute',
  balancePerMinute: 'open_balance_api.balance_rate_limit_per_minute',
  failureLockThreshold: 'open_balance_api.failure_lock_threshold',
  failureLockMinutes: 'open_balance_api.failure_lock_minutes',
} as const

type NumericFieldKey = Exclude<keyof typeof OPTION_KEYS, 'enabled'>

const NUMERIC_FIELDS: { key: NumericFieldKey; label: string; hint: string }[] =
  [
    {
      key: 'exchangePerMinute',
      label: 'Exchanges per application per minute',
      hint: 'Applications may override this with their own limit.',
    },
    {
      key: 'exchangeIpPerMinute',
      label: 'Exchanges per source IP per minute',
      hint: 'Pre-authentication backstop against anonymous floods.',
    },
    {
      key: 'balancePerMinute',
      label: 'Balance reads per credential per minute',
      hint: 'Bounds partner frontends that poll without backing off.',
    },
    {
      key: 'failureLockThreshold',
      label: 'Failed attempts before lockout',
      hint: 'Counted per application and username. The main defense against credential stuffing.',
    },
    {
      key: 'failureLockMinutes',
      label: 'Lockout duration (minutes)',
      hint: 'How long a locked out username stays blocked for that application.',
    },
  ]

type NumericValues = Record<NumericFieldKey, string>

const emptyNumericValues: NumericValues = {
  exchangePerMinute: '',
  exchangeIpPerMinute: '',
  balancePerMinute: '',
  failureLockThreshold: '',
  failureLockMinutes: '',
}

export function OpenApiSettingsCard() {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [enabled, setEnabled] = useState(false)
  const [values, setValues] = useState<NumericValues>(emptyNumericValues)

  const optionsQuery = useQuery({
    queryKey: ['system-options'],
    queryFn: getSystemOptions,
  })

  useEffect(() => {
    const options = optionsQuery.data?.data
    if (!options) return
    const byKey = new Map(options.map((option) => [option.key, option.value]))
    setEnabled(byKey.get(OPTION_KEYS.enabled) === 'true')
    setValues({
      exchangePerMinute: byKey.get(OPTION_KEYS.exchangePerMinute) ?? '',
      exchangeIpPerMinute: byKey.get(OPTION_KEYS.exchangeIpPerMinute) ?? '',
      balancePerMinute: byKey.get(OPTION_KEYS.balancePerMinute) ?? '',
      failureLockThreshold: byKey.get(OPTION_KEYS.failureLockThreshold) ?? '',
      failureLockMinutes: byKey.get(OPTION_KEYS.failureLockMinutes) ?? '',
    })
  }, [optionsQuery.data])

  const handleToggle = (next: boolean) => {
    setEnabled(next)
    updateOption.mutate({ key: OPTION_KEYS.enabled, value: next })
  }

  const handleSaveLimits = () => {
    for (const field of NUMERIC_FIELDS) {
      const parsed = Number(values[field.key])
      if (!Number.isFinite(parsed) || parsed < 0) continue
      updateOption.mutate({ key: OPTION_KEYS[field.key], value: parsed })
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Open API settings')}</CardTitle>
        <CardDescription>
          {t(
            'The balance open API lets partner sites exchange a user password for a read-only balance credential. It stays off until you enable it.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-5'>
        <div className='flex items-center justify-between gap-4'>
          <div className='space-y-1'>
            <Label htmlFor='open-api-enabled'>{t('Enable the open API')}</Label>
            <p className='text-muted-foreground text-sm'>
              {t(
                'While disabled, every endpoint under /api/open/v1 answers 503.'
              )}
            </p>
          </div>
          <Switch
            id='open-api-enabled'
            checked={enabled}
            disabled={optionsQuery.isLoading || updateOption.isPending}
            onCheckedChange={handleToggle}
          />
        </div>

        <div className='grid gap-4 sm:grid-cols-2'>
          {NUMERIC_FIELDS.map((field) => (
            <div key={field.key} className='space-y-1.5'>
              <Label htmlFor={`open-api-${field.key}`}>{t(field.label)}</Label>
              <Input
                id={`open-api-${field.key}`}
                type='number'
                min={0}
                value={values[field.key]}
                disabled={optionsQuery.isLoading}
                onChange={(event) =>
                  setValues((previous) => ({
                    ...previous,
                    [field.key]: event.target.value,
                  }))
                }
              />
              <p className='text-muted-foreground text-xs'>{t(field.hint)}</p>
            </div>
          ))}
        </div>

        <div className='flex justify-end'>
          <Button
            onClick={handleSaveLimits}
            disabled={optionsQuery.isLoading || updateOption.isPending}
          >
            {updateOption.isPending ? (
              <Loader2 className='animate-spin' />
            ) : null}
            {t('Save limits')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
