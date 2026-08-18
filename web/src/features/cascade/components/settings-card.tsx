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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Textarea } from '@/components/ui/textarea'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { updateCascadeSettingOption } from '../api'
import type { CascadeSetting } from '../types'

type SettingsForm = {
  enabled: boolean
  probe_enabled: boolean
  watermark_enabled: boolean
  incomplete_stream_as_fault: boolean
  failure_threshold: string
  cooldown_seconds: string
  probe_interval_seconds: string
  recovery_success_count: string
  max_attempts_per_request: string
  extra_fault_status_codes: string
  extra_fault_keywords: string
  ignore_fault_keywords: string
}

function toForm(setting: CascadeSetting): SettingsForm {
  return {
    enabled: setting.enabled,
    probe_enabled: setting.probe_enabled,
    watermark_enabled: setting.watermark_enabled,
    incomplete_stream_as_fault: setting.incomplete_stream_as_fault,
    failure_threshold: String(setting.failure_threshold),
    cooldown_seconds: String(setting.cooldown_seconds),
    probe_interval_seconds: String(setting.probe_interval_seconds),
    recovery_success_count: String(setting.recovery_success_count),
    max_attempts_per_request: String(setting.max_attempts_per_request),
    extra_fault_status_codes: (setting.extra_fault_status_codes ?? []).join(
      ', '
    ),
    extra_fault_keywords: (setting.extra_fault_keywords ?? []).join('\n'),
    ignore_fault_keywords: (setting.ignore_fault_keywords ?? []).join('\n'),
  }
}

function parseKeywords(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}

export function CascadeSettingsCard({ setting }: { setting: CascadeSetting }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { auth } = useAuthStore()
  const isRoot = (auth.user?.role ?? 0) >= ROLE.SUPER_ADMIN

  const [form, setForm] = useState<SettingsForm>(() => toForm(setting))

  const saveMutation = useMutation({
    mutationFn: async () => {
      const statusCodes = form.extra_fault_status_codes
        .split(/[,，\s]+/)
        .map((item) => item.trim())
        .filter(Boolean)
        .map((item) => Number.parseInt(item, 10))
        .filter((code) => Number.isInteger(code) && code >= 100 && code <= 599)

      const updates: { key: string; value: string | boolean | number }[] = [
        { key: 'enabled', value: form.enabled },
        { key: 'probe_enabled', value: form.probe_enabled },
        { key: 'watermark_enabled', value: form.watermark_enabled },
        {
          key: 'incomplete_stream_as_fault',
          value: form.incomplete_stream_as_fault,
        },
        {
          key: 'failure_threshold',
          value: Number.parseInt(form.failure_threshold, 10) || 1,
        },
        {
          key: 'cooldown_seconds',
          value: Number.parseInt(form.cooldown_seconds, 10) || 120,
        },
        {
          key: 'probe_interval_seconds',
          value: Number.parseInt(form.probe_interval_seconds, 10) || 60,
        },
        {
          key: 'recovery_success_count',
          value: Number.parseInt(form.recovery_success_count, 10) || 3,
        },
        {
          key: 'max_attempts_per_request',
          value: Number.parseInt(form.max_attempts_per_request, 10) || 0,
        },
        {
          key: 'extra_fault_status_codes',
          value: JSON.stringify(statusCodes),
        },
        {
          key: 'extra_fault_keywords',
          value: JSON.stringify(parseKeywords(form.extra_fault_keywords)),
        },
        {
          key: 'ignore_fault_keywords',
          value: JSON.stringify(parseKeywords(form.ignore_fault_keywords)),
        },
      ]
      for (const update of updates) {
        const res = await updateCascadeSettingOption(update.key, update.value)
        if (!res.success) {
          throw new Error(res.message || update.key)
        }
      }
    },
    onSuccess: () => {
      toast.success(t('Settings saved'))
      queryClient.invalidateQueries({ queryKey: ['cascade', 'overview'] })
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('Save failed'))
    },
  })

  const numberField = (
    key:
      | 'failure_threshold'
      | 'cooldown_seconds'
      | 'probe_interval_seconds'
      | 'recovery_success_count'
      | 'max_attempts_per_request',
    label: string,
    hint?: string
  ) => (
    <div className='space-y-1.5'>
      <Label htmlFor={`cascade-${key}`}>{label}</Label>
      <Input
        id={`cascade-${key}`}
        inputMode='numeric'
        value={form[key]}
        onChange={(e) =>
          setForm((prev) => ({ ...prev, [key]: e.target.value }))
        }
      />
      {hint && <p className='text-muted-foreground text-xs'>{hint}</p>}
    </div>
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Cascade Settings')}</CardTitle>
        <CardDescription>
          {t(
            'Fault errors (429/401/403/5xx/network) trip the channel and spill traffic to the next one; 400-class errors are returned to the client as-is.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='flex flex-wrap gap-6'>
          <div className='flex items-center gap-2'>
            <Switch
              id='cascade-enabled'
              checked={form.enabled}
              onCheckedChange={(checked) =>
                setForm((prev) => ({ ...prev, enabled: checked }))
              }
            />
            <Label htmlFor='cascade-enabled'>{t('Enable Cascade')}</Label>
          </div>
          <div className='flex items-center gap-2'>
            <Switch
              id='cascade-probe'
              checked={form.probe_enabled}
              onCheckedChange={(checked) =>
                setForm((prev) => ({ ...prev, probe_enabled: checked }))
              }
            />
            <Label htmlFor='cascade-probe'>{t('Probe tripped channels')}</Label>
          </div>
          <div className='flex items-center gap-2'>
            <Switch
              id='cascade-watermark'
              checked={form.watermark_enabled}
              onCheckedChange={(checked) =>
                setForm((prev) => ({ ...prev, watermark_enabled: checked }))
              }
            />
            <Label htmlFor='cascade-watermark'>
              {t('Enable RPM watermark')}
            </Label>
          </div>
          <div className='flex items-center gap-2'>
            <Switch
              id='cascade-incomplete-stream'
              checked={form.incomplete_stream_as_fault}
              onCheckedChange={(checked) =>
                setForm((prev) => ({
                  ...prev,
                  incomplete_stream_as_fault: checked,
                }))
              }
            />
            <Label htmlFor='cascade-incomplete-stream'>
              {t('Count silent stream breaks as faults')}
            </Label>
          </div>
        </div>

        <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
          {numberField(
            'failure_threshold',
            t('Failure threshold'),
            t('Consecutive fault errors before tripping')
          )}
          {numberField(
            'cooldown_seconds',
            t('Cooldown seconds'),
            t('Half-open fallback window when probing is off')
          )}
          {numberField('probe_interval_seconds', t('Probe interval seconds'))}
          {numberField(
            'recovery_success_count',
            t('Recovery success count'),
            t('Consecutive successes required to restore')
          )}
          {numberField(
            'max_attempts_per_request',
            t('Max attempts per request'),
            t('0 = try every channel in the group')
          )}
        </div>

        <div className='grid gap-4 lg:grid-cols-3'>
          <div className='space-y-1.5'>
            <Label htmlFor='cascade-extra-codes'>
              {t('Extra fault status codes')}
            </Label>
            <Input
              id='cascade-extra-codes'
              placeholder='418, 424'
              value={form.extra_fault_status_codes}
              onChange={(e) =>
                setForm((prev) => ({
                  ...prev,
                  extra_fault_status_codes: e.target.value,
                }))
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t('Comma separated; also treated as channel faults')}
            </p>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='cascade-extra-keywords'>
              {t('Extra fault keywords')}
            </Label>
            <Textarea
              id='cascade-extra-keywords'
              rows={3}
              value={form.extra_fault_keywords}
              onChange={(e) =>
                setForm((prev) => ({
                  ...prev,
                  extra_fault_keywords: e.target.value,
                }))
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t('One per line; matching errors are treated as channel faults')}
            </p>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='cascade-ignore-keywords'>
              {t('Ignore fault keywords')}
            </Label>
            <Textarea
              id='cascade-ignore-keywords'
              rows={3}
              value={form.ignore_fault_keywords}
              onChange={(e) =>
                setForm((prev) => ({
                  ...prev,
                  ignore_fault_keywords: e.target.value,
                }))
              }
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'One per line; matching errors never trip the channel (highest precedence)'
              )}
            </p>
          </div>
        </div>

        <div className='flex items-center gap-3'>
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={!isRoot || saveMutation.isPending}
          >
            {t('Save Settings')}
          </Button>
          <Button
            variant='outline'
            onClick={() => setForm(toForm(setting))}
            disabled={saveMutation.isPending}
          >
            {t('Reset')}
          </Button>
          {!isRoot && (
            <span className='text-muted-foreground text-xs'>
              {t('Only the root user can modify these settings')}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
