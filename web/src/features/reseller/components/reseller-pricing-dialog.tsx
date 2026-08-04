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
import { Clock3, Loader2, Save } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { deletePricing, updatePricing } from '../api'
import type {
  ResellerCustomer,
  ResellerPricingResponse,
  ResellerPricingRule,
} from '../types'

interface GroupInfo {
  desc: string
  ratio: number | string
}

interface ResellerPricingDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  customer?: ResellerCustomer | null
  groups: Record<string, GroupInfo>
  loadPricing: () => Promise<ResellerPricingResponse>
  onCompleted: () => void | Promise<void>
}

const requestedMultiplier = (rule?: ResellerPricingRule) =>
  rule?.pending_multiplier_bps || rule?.current_multiplier_bps || 10000

export function ResellerPricingDialog({
  open,
  onOpenChange,
  customer,
  groups,
  loadPricing,
  onCompleted,
}: ResellerPricingDialogProps) {
  const { t } = useTranslation()
  const [pricing, setPricing] = useState<ResellerPricingResponse | null>(null)
  const [values, setValues] = useState<Record<string, number>>({})
  const [overrides, setOverrides] = useState<Record<string, boolean>>({})
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  const groupEntries = useMemo(
    () =>
      Object.entries(groups).sort(([left], [right]) =>
        left.localeCompare(right)
      ),
    [groups]
  )

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const next = await loadPricing()
      setPricing(next)
      const nextValues: Record<string, number> = {
        '': requestedMultiplier(next.rules['']),
      }
      const nextOverrides: Record<string, boolean> = {}
      for (const [name] of groupEntries) {
        nextValues[name] = requestedMultiplier(next.rules[name])
        nextOverrides[name] = Boolean(next.rules[name])
      }
      setValues(nextValues)
      setOverrides(nextOverrides)
    } finally {
      setLoading(false)
    }
  }, [groupEntries, loadPricing])

  useEffect(() => {
    if (open) void refresh()
  }, [open, refresh])

  const save = async () => {
    if (!pricing) return
    setSaving(true)
    let version = pricing.pricing_version
    const owner = customer ? customer.binding_id : 'default'
    try {
      const scopes = ['', ...groupEntries.map(([name]) => name)]
      for (const groupName of scopes) {
        const current = pricing.rules[groupName]
        const enabled = groupName === '' || overrides[groupName]
        if (!enabled && current) {
          const response = await deletePricing(owner, {
            group_name: groupName,
            expected_version: version,
          })
          version = response.data.pricing_version
          continue
        }
        if (!enabled) continue
        const multiplierBps = Math.round(values[groupName] || 0)
        if (
          multiplierBps < 10000 ||
          multiplierBps > 100000 ||
          (current && requestedMultiplier(current) === multiplierBps)
        ) {
          continue
        }
        const response = await updatePricing(owner, {
          group_name: groupName,
          multiplier_bps: multiplierBps,
          expected_version: version,
        })
        version = (response.data as unknown as { pricing_version: number })
          .pricing_version
      }
      toast.success(t('Pricing updated'))
      await onCompleted()
      onOpenChange(false)
    } catch (error: unknown) {
      const code = (
        error as { response?: { data?: { data?: { code?: string } } } }
      )?.response?.data?.data?.code
      if (code === 'RESELLER_VERSION_CONFLICT') {
        toast.error(
          t('Pricing changed elsewhere. The latest version has been reloaded.')
        )
        await refresh()
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={customer ? t('Customer pricing') : t('Default pricing')}
      description={
        customer
          ? t(
              'Customer rules override reseller defaults for this direct customer.'
            )
          : t(
              'Set the overall multiplier and optional overrides for each platform group.'
            )
      }
      contentClassName='sm:max-w-4xl'
      contentHeight='min(68vh, 720px)'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={save} disabled={loading || saving || !pricing}>
            {saving ? <Loader2 className='animate-spin' /> : <Save />}
            {t('Save pricing')}
          </Button>
        </>
      }
    >
      {loading || !pricing ? (
        <div className='grid min-h-48 place-items-center'>
          <Loader2 className='text-muted-foreground size-5 animate-spin' />
        </div>
      ) : (
        <div className='space-y-4'>
          <Alert>
            <Clock3 />
            <AlertTitle>{t('Pricing activation')}</AlertTitle>
            <AlertDescription>
              {t(
                'First-time settings and decreases apply immediately. Increases take effect after 24 hours.'
              )}
            </AlertDescription>
          </Alert>

          <div className='grid gap-2 border-b pb-4 sm:grid-cols-[minmax(0,1fr)_180px] sm:items-end'>
            <div>
              <Label htmlFor='reseller-overall-multiplier'>
                {t('Overall multiplier')}
              </Label>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Used by every group that remains inherited.')}
              </p>
            </div>
            <MultiplierInput
              id='reseller-overall-multiplier'
              value={values['']}
              onChange={(value) =>
                setValues((current) => ({ ...current, '': value }))
              }
            />
          </div>

          <div className='divide-y rounded-md border'>
            {groupEntries.length === 0 ? (
              <div className='text-muted-foreground p-4 text-sm'>
                {t('No platform groups available.')}
              </div>
            ) : (
              groupEntries.map(([name, info]) => {
                const inherited = !overrides[name]
                const pendingAt = pricing.rules[name]?.pending_effective_at
                return (
                  <div
                    key={name}
                    className='grid gap-3 p-3 sm:grid-cols-[minmax(0,1fr)_auto_150px] sm:items-center'
                  >
                    <div className='min-w-0'>
                      <div className='truncate font-medium'>
                        {info.desc || name}
                      </div>
                      <div className='text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-2 text-xs'>
                        <span>{name}</span>
                        <span>
                          {t('Platform ratio')}: {String(info.ratio)}
                        </span>
                        {pendingAt ? (
                          <span className='text-warning'>
                            {t('Increase pending')}
                          </span>
                        ) : null}
                      </div>
                    </div>
                    <label className='flex items-center gap-2 text-sm'>
                      <Switch
                        checked={inherited}
                        onCheckedChange={(checked) =>
                          setOverrides((current) => ({
                            ...current,
                            [name]: !checked,
                          }))
                        }
                      />
                      <span>{t('Inherit overall')}</span>
                    </label>
                    <MultiplierInput
                      value={inherited ? values[''] : values[name]}
                      disabled={inherited}
                      onChange={(value) =>
                        setValues((current) => ({ ...current, [name]: value }))
                      }
                    />
                  </div>
                )
              })
            )}
          </div>
        </div>
      )}
    </Dialog>
  )
}

function MultiplierInput({
  id,
  value,
  disabled,
  onChange,
}: {
  id?: string
  value?: number
  disabled?: boolean
  onChange: (value: number) => void
}) {
  return (
    <div className='relative'>
      <Input
        id={id}
        type='number'
        min={1}
        max={10}
        step={0.0001}
        value={(value || 10000) / 10000}
        disabled={disabled}
        onChange={(event) =>
          onChange(Math.round(Number(event.target.value) * 10000))
        }
        className='pr-8 tabular-nums'
      />
      <span className='text-muted-foreground pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs'>
        x
      </span>
    </div>
  )
}
