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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, UserPlus } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { ResellerCustomer, ResellerPage } from '@/features/reseller/types'
import { unbindUserFromReseller } from '@/features/users/api'

import { listResellerCustomers, listResellers } from './api'
import { AddCustomerDialog } from './components/add-customer-dialog'
import { OpenResellerDialog } from './components/open-reseller-dialog'
import { ResellerCustomersPanel } from './components/reseller-customers-panel'
import { ResellerRosterTable } from './components/reseller-roster-table'
import type { OpenResellerResult, ResellerRosterItem } from './types'

const PAGE_SIZE = 20
const SEARCH_DEBOUNCE_MS = 300
const ROSTER_QUERY_KEY = ['reseller-admin', 'roster'] as const
const CUSTOMERS_QUERY_KEY = ['reseller-admin', 'customers'] as const

function emptyPage<T>(): ResellerPage<T> {
  return { page: 1, page_size: PAGE_SIZE, total: 0, items: [] }
}

export function ResellerAdmin() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [keyword, setKeyword] = useState('')
  const [debouncedKeyword, setDebouncedKeyword] = useState('')
  const [rosterPage, setRosterPage] = useState(1)
  const [selected, setSelected] = useState<ResellerRosterItem | null>(null)
  const [customersPage, setCustomersPage] = useState(1)
  const [addOpen, setAddOpen] = useState(false)
  const [openResellerOpen, setOpenResellerOpen] = useState(false)
  // A freshly opened center is selected once the roster reports it, so the
  // operator can go straight on to handing it its first customers.
  const [pendingSelectUserId, setPendingSelectUserId] = useState(0)
  const [unbindTarget, setUnbindTarget] = useState<ResellerCustomer | null>(
    null
  )

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedKeyword(keyword.trim())
      setRosterPage(1)
    }, SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [keyword])

  const rosterQuery = useQuery({
    queryKey: [...ROSTER_QUERY_KEY, rosterPage, debouncedKeyword],
    queryFn: async () => {
      const result = await listResellers(
        rosterPage,
        PAGE_SIZE,
        debouncedKeyword
      )
      if (!result.success) {
        throw new Error(result.message || t('Failed to load resellers'))
      }
      return result.data
    },
  })

  const customersQuery = useQuery({
    queryKey: [...CUSTOMERS_QUERY_KEY, selected?.user_id ?? 0, customersPage],
    queryFn: async () => {
      if (!selected) return emptyPage<ResellerCustomer>()
      const result = await listResellerCustomers(
        selected.user_id,
        customersPage,
        PAGE_SIZE
      )
      if (!result.success) {
        throw new Error(result.message || t('Failed to load customers'))
      }
      return result.data
    },
    enabled: selected !== null,
  })

  useEffect(() => {
    if (pendingSelectUserId === 0) return
    const match = rosterQuery.data?.items.find(
      (item) => item.user_id === pendingSelectUserId
    )
    if (!match) return
    setSelected(match)
    setCustomersPage(1)
    setPendingSelectUserId(0)
  }, [pendingSelectUserId, rosterQuery.data])

  const refreshAll = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ['reseller-admin'] })
  }, [queryClient])

  // Searching by the target's username is what brings a brand new center into
  // the current roster page; the pending id then picks it out of the results.
  const handleResellerOpened = useCallback(
    async (result: OpenResellerResult) => {
      setKeyword(result.username)
      setPendingSelectUserId(result.user_id)
      await refreshAll()
    },
    [refreshAll]
  )

  const unbindMutation = useMutation({
    mutationFn: async (customer: ResellerCustomer) => {
      const result = await unbindUserFromReseller(customer.customer_id)
      if (!result.success) {
        throw new Error(result.message || t('Failed to unbind the customer'))
      }
    },
    onSuccess: async () => {
      toast.success(t('Customer unbound'))
      setUnbindTarget(null)
      await refreshAll()
    },
    onError: (error) =>
      toast.error(
        error instanceof Error
          ? error.message
          : t('Failed to unbind the customer')
      ),
  })

  const roster = rosterQuery.data ?? emptyPage<ResellerRosterItem>()
  const customers = customersQuery.data ?? emptyPage<ResellerCustomer>()

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Resellers')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button size='sm' onClick={() => setOpenResellerOpen(true)}>
            <UserPlus />
            <span className='hidden sm:inline'>
              {t('Open a reseller center')}
            </span>
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={refreshAll}
            disabled={rosterQuery.isFetching}
            aria-label={t('Refresh reseller data')}
          >
            <RefreshCw
              className={rosterQuery.isFetching ? 'animate-spin' : ''}
            />
            <span className='hidden sm:inline'>{t('Refresh')}</span>
          </Button>
        </SectionPageLayout.Actions>

        {/* SectionPageLayout renders named slots only; anything passed as a
            bare child is dropped, so the body must live inside Content and the
            dialogs must sit outside the layout entirely. */}
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <ResellerRosterTable
              page={roster}
              selectedUserId={selected?.user_id ?? 0}
              searchInput={
                <Input
                  className='h-8 w-56'
                  value={keyword}
                  onChange={(event) => setKeyword(event.target.value)}
                  placeholder={t('Search resellers')}
                  aria-label={t('Search resellers')}
                />
              }
              onSelect={(reseller) => {
                setSelected(reseller)
                setCustomersPage(1)
              }}
              onPageChange={setRosterPage}
            />

            {selected ? (
              <ResellerCustomersPanel
                reseller={selected}
                page={customers}
                onPageChange={setCustomersPage}
                onAdd={() => setAddOpen(true)}
                onUnbind={setUnbindTarget}
                unbindingId={
                  unbindMutation.isPending
                    ? (unbindTarget?.customer_id ?? 0)
                    : 0
                }
              />
            ) : null}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <OpenResellerDialog
        open={openResellerOpen}
        onOpenChange={setOpenResellerOpen}
        onOpened={handleResellerOpened}
      />

      <AddCustomerDialog
        reseller={selected}
        open={addOpen}
        onOpenChange={setAddOpen}
        onBound={refreshAll}
      />

      <AlertDialog
        open={unbindTarget !== null}
        onOpenChange={(open) => {
          if (!open) setUnbindTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Unbind this customer?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'The account keeps its balance and history but stops belonging to this reseller. Customer-level pricing set by the reseller is dropped.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={unbindMutation.isPending}
              onClick={() =>
                unbindTarget && unbindMutation.mutate(unbindTarget)
              }
            >
              {t('Unbind')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
