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
import { Loader2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { updateCustomerNote } from '../api'
import type { ResellerCustomer } from '../types'

const NOTE_MAX_LENGTH = 255

interface ResellerCustomerNoteDialogProps {
  customer: ResellerCustomer | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onCompleted: () => void | Promise<void>
}

export function ResellerCustomerNoteDialog({
  customer,
  open,
  onOpenChange,
  onCompleted,
}: ResellerCustomerNoteDialogProps) {
  const { t } = useTranslation()
  const [note, setNote] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (open) setNote(customer?.note || '')
  }, [customer, open])

  const submit = async () => {
    if (!customer) return
    setSubmitting(true)
    try {
      await updateCustomerNote(customer.binding_id, note.trim())
      toast.success(t('Customer note saved'))
      await onCompleted()
      onOpenChange(false)
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Customer note')}
      description={t(
        'This note is private to you and never shown to the customer.'
      )}
      contentClassName='sm:max-w-md'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            {t('Cancel')}
          </Button>
          <Button onClick={submit} disabled={submitting}>
            {submitting && <Loader2 className='animate-spin' />}
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-1.5'>
        <Label>
          {t('Note for {{username}}', { username: customer?.username || '' })}
        </Label>
        <Input
          value={note}
          maxLength={NOTE_MAX_LENGTH}
          autoComplete='off'
          placeholder={t('Leave blank to clear the note')}
          onChange={(event) => setNote(event.target.value)}
        />
      </div>
    </Dialog>
  )
}
