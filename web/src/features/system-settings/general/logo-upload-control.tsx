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
import { Upload } from 'lucide-react'
import { type ChangeEvent, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { uploadLogo } from '../api'

const MAX_LOGO_SIZE = 2 * 1024 * 1024

// Keep in sync with `allowedLogoExts` in controller/option.go
const ACCEPTED_LOGO_EXTENSIONS = [
  '.png',
  '.jpg',
  '.jpeg',
  '.gif',
  '.svg',
  '.ico',
  '.webp',
]

type LogoUploadControlProps = {
  value: string
  onUploaded: (url: string) => void
}

export function LogoUploadControl(props: LogoUploadControlProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const setSystemConfig = useSystemConfigStore((s) => s.setConfig)
  // The backend always stores the logo under the same file name, so a fresh
  // upload keeps the previous URL and would otherwise render from cache.
  const [uploadedPreview, setUploadedPreview] = useState<{
    url: string
    version: number
  } | null>(null)

  const upload = useMutation({
    mutationFn: (file: File) => uploadLogo(file),
    onSuccess: (data) => {
      // Business failures are already surfaced by the axios response interceptor
      if (!data.success || !data.data) return

      const url = data.data
      props.onUploaded(url)
      setSystemConfig({ logo: url })
      setUploadedPreview({ url, version: Date.now() })

      // The option itself is written by the upload endpoint; mark the cached
      // settings stale without refetching so unsaved edits survive.
      queryClient.invalidateQueries({
        queryKey: ['system-options'],
        refetchType: 'none',
      })
      queryClient.invalidateQueries({ queryKey: ['status'] })
      try {
        window.localStorage.removeItem('status')
      } catch {
        /* empty */
      }

      toast.success(t('Logo updated successfully'))
    },
  })

  const handleFileChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return

    const extension = file.name.slice(file.name.lastIndexOf('.')).toLowerCase()
    if (!ACCEPTED_LOGO_EXTENSIONS.includes(extension)) {
      toast.error(
        t('Only png, jpg, jpeg, gif, svg, ico and webp images are supported')
      )
      return
    }

    if (file.size > MAX_LOGO_SIZE) {
      toast.error(t('Logo image must be 2 MB or smaller'))
      return
    }

    upload.mutate(file)
  }

  const previewSrc =
    uploadedPreview && uploadedPreview.url === props.value
      ? `${props.value}?t=${uploadedPreview.version}`
      : props.value

  return (
    <div className='flex items-center gap-3'>
      {props.value ? (
        <img
          src={previewSrc}
          alt={t('Logo')}
          className='h-10 w-10 rounded border object-contain p-1'
        />
      ) : (
        <div className='bg-muted text-muted-foreground flex h-10 w-10 items-center justify-center rounded border text-xs'>
          {t('Logo')}
        </div>
      )}
      <input
        ref={fileInputRef}
        type='file'
        accept={ACCEPTED_LOGO_EXTENSIONS.join(',')}
        className='hidden'
        onChange={handleFileChange}
      />
      <Button
        type='button'
        variant='outline'
        size='sm'
        onClick={() => fileInputRef.current?.click()}
        disabled={upload.isPending}
      >
        <Upload className='mr-1.5 h-3.5 w-3.5' />
        {upload.isPending ? t('Uploading...') : t('Upload logo')}
      </Button>
    </div>
  )
}
