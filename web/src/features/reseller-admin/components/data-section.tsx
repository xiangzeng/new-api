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
import { ChevronLeft, ChevronRight } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { ResellerPage } from '@/features/reseller/types'

/**
 * Bordered section with a counted title, an empty placeholder and paging.
 * Mirrors the shell the reseller center uses so the operator screen and the
 * reseller's own screen read as the same product.
 */
export function DataSection(props: {
  title: string
  count: number
  action?: ReactNode
  empty: boolean
  emptyText: string
  page: ResellerPage<unknown>
  onPageChange: (page: number) => void
  children: ReactNode
}) {
  return (
    <section className='overflow-hidden rounded-md border'>
      <div className='flex min-h-12 flex-wrap items-center justify-between gap-2 border-b px-3 py-2'>
        <div className='flex items-center gap-2'>
          <h3 className='font-medium'>{props.title}</h3>
          <Badge variant='secondary'>{props.count}</Badge>
        </div>
        {props.action}
      </div>
      {props.empty ? (
        <div className='text-muted-foreground grid min-h-40 place-items-center p-4 text-sm'>
          {props.emptyText}
        </div>
      ) : (
        props.children
      )}
      <DataPagination page={props.page} onPageChange={props.onPageChange} />
    </section>
  )
}

function DataPagination(props: {
  page: ResellerPage<unknown>
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pageCount = Math.max(
    1,
    Math.ceil(props.page.total / Math.max(1, props.page.page_size))
  )
  if (pageCount === 1 && props.page.page === 1) return null
  return (
    <div className='flex min-h-12 items-center justify-between gap-3 border-t px-3 py-2'>
      <span className='text-muted-foreground text-xs tabular-nums'>
        {t('Page {{page}} of {{total}}', {
          page: props.page.page,
          total: pageCount,
        })}
      </span>
      <div className='flex gap-1'>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={props.page.page <= 1}
          onClick={() => props.onPageChange(props.page.page - 1)}
          aria-label={t('Previous page')}
        >
          <ChevronLeft />
        </Button>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={props.page.page >= pageCount}
          onClick={() => props.onPageChange(props.page.page + 1)}
          aria-label={t('Next page')}
        >
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
