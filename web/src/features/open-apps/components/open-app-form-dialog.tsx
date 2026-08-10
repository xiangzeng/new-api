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
import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import { OPEN_APP_STATUS, type OpenApp, type OpenAppRequest } from '../types'

// The rate limit stays a string through the form so the input keeps a stable
// value type; it is parsed once on submit.
const openAppFormSchema = z.object({
  name: z.string().trim().min(1).max(64),
  allowed_ips: z.string(),
  enabled: z.boolean(),
  exchange_rate_limit: z
    .string()
    .refine((value) => /^\d*$/.test(value.trim()), {
      message: 'Enter a whole number of requests per minute, or 0.',
    }),
})

type OpenAppFormValues = z.infer<typeof openAppFormSchema>

const emptyOpenAppForm: OpenAppFormValues = {
  name: '',
  allowed_ips: '',
  enabled: true,
  exchange_rate_limit: '0',
}

interface OpenAppFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** null creates a new application, otherwise the row being edited. */
  app: OpenApp | null
  onSubmit: (values: OpenAppRequest) => void
  pending: boolean
}

export function OpenAppFormDialog(props: OpenAppFormDialogProps) {
  const { t } = useTranslation()
  const form = useForm<OpenAppFormValues>({
    resolver: zodResolver(openAppFormSchema),
    defaultValues: emptyOpenAppForm,
  })

  useEffect(() => {
    if (!props.open) return
    if (!props.app) {
      form.reset(emptyOpenAppForm)
      return
    }
    form.reset({
      name: props.app.name,
      allowed_ips: props.app.allowed_ips,
      enabled: props.app.status === OPEN_APP_STATUS.ENABLED,
      exchange_rate_limit: String(props.app.exchange_rate_limit),
    })
  }, [form, props.app, props.open])

  const handleSubmit = form.handleSubmit((values) => {
    props.onSubmit({
      name: values.name.trim(),
      allowed_ips: values.allowed_ips.trim(),
      status: values.enabled
        ? OPEN_APP_STATUS.ENABLED
        : OPEN_APP_STATUS.DISABLED,
      exchange_rate_limit: Number(values.exchange_rate_limit.trim() || '0'),
    })
  })

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.app ? t('Edit Application') : t('New Application')}
      description={t(
        'Partner sites use these credentials to exchange a user password for a read-only balance credential.'
      )}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={props.pending}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={props.pending}>
            {props.pending ? <Loader2 className='animate-spin' /> : null}
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form className='space-y-4' onSubmit={handleSubmit}>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Application name')}</FormLabel>
                <FormControl>
                  <Input {...field} placeholder={t('Partner site')} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='allowed_ips'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Allowed source IPs')}</FormLabel>
                <FormControl>
                  <Textarea
                    {...field}
                    rows={3}
                    className='font-mono text-xs'
                    placeholder={'203.0.113.0/24\n198.51.100.9'}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'One IP or CIDR per line. Leave empty to allow any source. Requests arrive from the partner backend, so this is their server address.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='exchange_rate_limit'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Exchange rate limit (per minute)')}</FormLabel>
                <FormControl>
                  <Input {...field} type='number' min={0} />
                </FormControl>
                <FormDescription>
                  {t('Set to 0 to use the global limit.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center justify-between gap-4'>
                <div className='space-y-1'>
                  <Label htmlFor='open-app-enabled'>{t('Enabled')}</Label>
                  <FormDescription>
                    {t(
                      'Disabling revokes every credential this application currently holds.'
                    )}
                  </FormDescription>
                </div>
                <FormControl>
                  <Switch
                    id='open-app-enabled'
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
