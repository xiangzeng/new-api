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
import {
  ArrowDownLeft,
  ArrowRightLeft,
  ArrowUpRight,
  BadgeDollarSign,
  ChevronLeft,
  ChevronRight,
  CircleDollarSign,
  Clock3,
  KeyRound,
  Loader2,
  LockKeyhole,
  RefreshCw,
  RotateCcwKey,
  Settings2,
  Share2,
  ShieldAlert,
  SlidersHorizontal,
  Ticket,
  UserRoundCog,
  Users,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { CopyButton } from '@/components/copy-button'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  SecureVerificationDialog,
  useSecureVerification,
} from '@/features/auth/secure-verification'
import { getUserGroups } from '@/lib/api'
import { formatQuota, formatTimestampToDate } from '@/lib/format'

import {
  enableResellerProfile,
  getCustomerPricing,
  getDefaultPricing,
  getResellerCustomers,
  getResellerInvitation,
  getResellerLedger,
  getResellerSecurity,
  getResellerStatus,
  getResellerTransfers,
  getResellerVoucherBatches,
  getResellerVouchers,
  rotateReceiveAddress,
} from './api'
import {
  ResellerActionDialog,
  type ResellerActionKind,
} from './components/reseller-action-dialog'
import { ResellerPricingDialog } from './components/reseller-pricing-dialog'
import { ResellerRevealDialog } from './components/reseller-reveal-dialog'
import type {
  ResellerCustomer,
  ResellerInvitation,
  ResellerLedgerItem,
  ResellerPage,
  ResellerPricingResponse,
  ResellerSecurityStatus,
  ResellerStatus,
  ResellerTransfer,
  ResellerVoucher,
  ResellerVoucherBatch,
} from './types'

const emptyPage = <T,>(): ResellerPage<T> => ({
  page: 1,
  page_size: 50,
  total: 0,
  items: [],
})

type GroupInfo = { desc: string; ratio: number | string }

export function ResellerCenter() {
  const { t } = useTranslation()
  const [status, setStatus] = useState<ResellerStatus | null>(null)
  const [invitation, setInvitation] = useState<ResellerInvitation | null>(null)
  const [security, setSecurity] = useState<ResellerSecurityStatus | null>(null)
  const [defaultPricing, setDefaultPricing] =
    useState<ResellerPricingResponse | null>(null)
  const [customers, setCustomers] =
    useState<ResellerPage<ResellerCustomer>>(emptyPage)
  const [transfers, setTransfers] =
    useState<ResellerPage<ResellerTransfer>>(emptyPage)
  const [ledger, setLedger] =
    useState<ResellerPage<ResellerLedgerItem>>(emptyPage)
  const [vouchers, setVouchers] =
    useState<ResellerPage<ResellerVoucher>>(emptyPage)
  const [batches, setBatches] =
    useState<ResellerPage<ResellerVoucherBatch>>(emptyPage)
  const [groups, setGroups] = useState<Record<string, GroupInfo>>({})
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [enabling, setEnabling] = useState(false)
  const [error, setError] = useState('')
  const [action, setAction] = useState<ResellerActionKind | null>(null)
  const [pricingOpen, setPricingOpen] = useState(false)
  const [pricingCustomer, setPricingCustomer] =
    useState<ResellerCustomer | null>(null)
  const [reveal, setReveal] = useState<{
    publicId: string
    batch: boolean
  } | null>(null)
  const rotateVerification = useSecureVerification()
  const initialized = useRef(false)

  const load = useCallback(
    async (background = false) => {
      if (background) setRefreshing(true)
      else setLoading(true)
      setError('')
      try {
        const nextStatus = await getResellerStatus()
        setStatus(nextStatus)
        if (!nextStatus.enabled) return
        const [
          nextInvitation,
          nextSecurity,
          nextPricing,
          nextCustomers,
          nextTransfers,
          nextLedger,
          nextVouchers,
          nextBatches,
          groupResponse,
        ] = await Promise.all([
          getResellerInvitation(),
          getResellerSecurity(),
          getDefaultPricing(),
          getResellerCustomers(customers.page),
          getResellerTransfers(transfers.page),
          getResellerLedger(ledger.page),
          getResellerVouchers(vouchers.page),
          getResellerVoucherBatches(batches.page),
          getUserGroups(),
        ])
        setInvitation(nextInvitation)
        setSecurity(nextSecurity)
        setDefaultPricing(nextPricing)
        setCustomers(nextCustomers)
        setTransfers(nextTransfers)
        setLedger(nextLedger)
        setVouchers(nextVouchers)
        setBatches(nextBatches)
        setGroups((groupResponse.data || {}) as Record<string, GroupInfo>)
      } catch (loadError: unknown) {
        setError(
          (loadError as { response?: { data?: { message?: string } } })
            ?.response?.data?.message || t('Failed to load reseller center')
        )
      } finally {
        setLoading(false)
        setRefreshing(false)
      }
    },
    [
      batches.page,
      customers.page,
      ledger.page,
      t,
      transfers.page,
      vouchers.page,
    ]
  )

  useEffect(() => {
    void load(initialized.current)
    initialized.current = true
  }, [load])

  const enable = async () => {
    setEnabling(true)
    try {
      await enableResellerProfile()
      toast.success(t('Reseller center enabled'))
      await load()
    } finally {
      setEnabling(false)
    }
  }

  const invitationUrl = useMemo(() => {
    if (!invitation || typeof window === 'undefined') return ''
    return `${window.location.origin}${invitation.path}`
  }, [invitation])

  const openCustomerPricing = (customer: ResellerCustomer) => {
    setPricingCustomer(customer)
    setPricingOpen(true)
  }

  const rotateAddress = async () => {
    await rotateVerification.startVerification(
      async (proof) => {
        const response = await rotateReceiveAddress(proof)
        toast.success(t('Receive address rotated'))
        await load(true)
        return response
      },
      {
        scope: 'reseller.receive_address.rotate',
        title: t('Rotate receive address'),
        description: t(
          'The old receive address will stop accepting new transfer previews.'
        ),
      }
    )
  }

  if (loading) return <ResellerLoading />

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Reseller Center')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {status?.enabled && (
          <Button
            variant='outline'
            size='sm'
            onClick={() => load(true)}
            disabled={refreshing}
            aria-label={t('Refresh reseller data')}
          >
            <RefreshCw className={refreshing ? 'animate-spin' : ''} />
            <span className='hidden sm:inline'>{t('Refresh')}</span>
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='mx-auto w-full max-w-7xl space-y-4'>
          {error && (
            <Alert variant='destructive'>
              <ShieldAlert />
              <AlertTitle>{t('Unable to load reseller center')}</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {!status?.enabled ? (
            <EnableResellerPanel onEnable={enable} loading={enabling} />
          ) : (
            <>
              {status.status === 'frozen' && (
                <Alert variant='destructive'>
                  <ShieldAlert />
                  <AlertTitle>{t('Reseller account frozen')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Pricing and outbound operations are unavailable until the account is restored.'
                    )}
                  </AlertDescription>
                </Alert>
              )}
              {security?.outbound_frozen && (
                <Alert>
                  <Clock3 />
                  <AlertTitle>
                    {t('Outbound operations temporarily frozen')}
                  </AlertTitle>
                  <AlertDescription>
                    {t('Transfers and user code issuance resume at {{time}}.', {
                      time: formatTimestampToDate(
                        security.outbound_frozen_until
                      ),
                    })}
                  </AlertDescription>
                </Alert>
              )}

              <SummaryBand status={status} />

              <Tabs defaultValue='overview' className='gap-3'>
                <div className='overflow-x-auto pb-1'>
                  <TabsList variant='line' className='min-w-max'>
                    <TabsTrigger value='overview'>
                      <BadgeDollarSign />
                      {t('Overview')}
                    </TabsTrigger>
                    <TabsTrigger value='customers'>
                      <Users />
                      {t('Customers')}
                    </TabsTrigger>
                    <TabsTrigger value='ledger'>
                      <CircleDollarSign />
                      {t('Ledger')}
                    </TabsTrigger>
                    <TabsTrigger value='transfers'>
                      <ArrowRightLeft />
                      {t('Transfers')}
                    </TabsTrigger>
                    <TabsTrigger value='vouchers'>
                      <Ticket />
                      {t('User Codes')}
                    </TabsTrigger>
                    <TabsTrigger value='security'>
                      <LockKeyhole />
                      {t('Security')}
                    </TabsTrigger>
                  </TabsList>
                </div>

                <TabsContent value='overview'>
                  <OverviewPanel
                    invitationUrl={invitationUrl}
                    defaultPricing={defaultPricing}
                    onPricing={() => {
                      setPricingCustomer(null)
                      setPricingOpen(true)
                    }}
                    onAction={setAction}
                  />
                </TabsContent>
                <TabsContent value='customers'>
                  <CustomersPanel
                    page={customers}
                    onPricing={openCustomerPricing}
                    onPageChange={(page) =>
                      setCustomers((current) => ({ ...current, page }))
                    }
                  />
                </TabsContent>
                <TabsContent value='ledger'>
                  <LedgerPanel
                    page={ledger}
                    onPageChange={(page) =>
                      setLedger((current) => ({ ...current, page }))
                    }
                  />
                </TabsContent>
                <TabsContent value='transfers'>
                  <TransfersPanel
                    page={transfers}
                    onTransfer={() => setAction('transfer')}
                    onPageChange={(page) =>
                      setTransfers((current) => ({ ...current, page }))
                    }
                  />
                </TabsContent>
                <TabsContent value='vouchers'>
                  <VouchersPanel
                    vouchers={vouchers}
                    batches={batches}
                    onIssue={() => setAction('voucher')}
                    onReveal={(publicId, batch) =>
                      setReveal({ publicId, batch })
                    }
                    onVoucherPageChange={(page) =>
                      setVouchers((current) => ({ ...current, page }))
                    }
                    onBatchPageChange={(page) =>
                      setBatches((current) => ({ ...current, page }))
                    }
                  />
                </TabsContent>
                <TabsContent value='security'>
                  <SecurityPanel
                    security={security}
                    receivePublicId={status.receive_public_id || ''}
                    onAction={setAction}
                    onRotate={rotateAddress}
                  />
                </TabsContent>
              </Tabs>
            </>
          )}
        </div>

        {action && (
          <ResellerActionDialog
            kind={action}
            open
            onOpenChange={(open) => {
              if (!open) setAction(null)
            }}
            onCompleted={() => load(true)}
          />
        )}
        <ResellerPricingDialog
          open={pricingOpen}
          onOpenChange={(open) => {
            setPricingOpen(open)
            if (!open) setPricingCustomer(null)
          }}
          customer={pricingCustomer}
          groups={groups}
          loadPricing={() =>
            pricingCustomer
              ? getCustomerPricing(pricingCustomer.binding_id)
              : getDefaultPricing()
          }
          onCompleted={() => load(true)}
        />
        {reveal && (
          <ResellerRevealDialog
            open
            onOpenChange={(open) => {
              if (!open) setReveal(null)
            }}
            publicId={reveal.publicId}
            batch={reveal.batch}
          />
        )}
        <SecureVerificationDialog
          open={rotateVerification.open}
          onOpenChange={(open) => {
            if (!open) rotateVerification.cancel()
          }}
          methods={rotateVerification.methods}
          state={rotateVerification.state}
          onVerify={async (method, code) => {
            await rotateVerification.executeVerification(method, code)
          }}
          onCancel={rotateVerification.cancel}
          onCodeChange={rotateVerification.setCode}
          onMethodChange={rotateVerification.switchMethod}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function ResellerLoading() {
  const { t } = useTranslation()
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Reseller Center')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto grid w-full max-w-7xl gap-4'>
          <Skeleton className='h-24 rounded-md' />
          <Skeleton className='h-9 w-full max-w-xl' />
          <Skeleton className='h-80 rounded-md' />
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function EnableResellerPanel({
  onEnable,
  loading,
}: {
  onEnable: () => void
  loading: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-card/30 grid min-h-[360px] place-items-center rounded-md border p-6 text-center'>
      <div className='max-w-md space-y-4'>
        <div className='bg-muted mx-auto grid size-12 place-items-center rounded-md'>
          <Share2 className='size-5' />
        </div>
        <div>
          <h3 className='text-lg font-semibold'>{t('Open Reseller Center')}</h3>
          <p className='text-muted-foreground mt-2 text-sm'>
            {t(
              'Bind direct customers, set relative pricing, and receive the settled price difference as internal quota.'
            )}
          </p>
        </div>
        <Button onClick={onEnable} disabled={loading}>
          {loading && <Loader2 className='animate-spin' />}
          {t('Enable reseller mode')}
        </Button>
      </div>
    </div>
  )
}

function SummaryBand({ status }: { status: ResellerStatus }) {
  const { t } = useTranslation()
  const stats = [
    [
      t('Available earnings'),
      formatQuota(status.available_commission_quota),
      CircleDollarSign,
    ],
    [
      t('Pending earnings'),
      formatQuota(status.pending_commission_quota),
      Clock3,
    ],
    [t('Direct customers'), String(status.customer_count), Users],
    [t('Sent in 24 hours'), `${status.outbound_used_24h} / 4000`, ArrowUpRight],
  ] as const
  return (
    <div className='bg-card/30 grid divide-y rounded-md border sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4'>
      {stats.map(([label, value, Icon]) => (
        <div key={label} className='flex min-w-0 items-center gap-3 px-4 py-3'>
          <Icon className='text-muted-foreground size-4 shrink-0' />
          <div className='min-w-0'>
            <div className='text-muted-foreground truncate text-xs'>
              {label}
            </div>
            <div className='mt-0.5 truncate font-semibold tabular-nums'>
              {value}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function OverviewPanel({
  invitationUrl,
  defaultPricing,
  onPricing,
  onAction,
}: {
  invitationUrl: string
  defaultPricing: ResellerPricingResponse | null
  onPricing: () => void
  onAction: (action: ResellerActionKind) => void
}) {
  const { t } = useTranslation()
  const overall = defaultPricing?.rules['']
  return (
    <div className='space-y-4'>
      <section className='space-y-2 rounded-md border p-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <h3 className='font-semibold'>{t('Customer invitation link')}</h3>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Only directly registered customers are attributed to this reseller account.'
              )}
            </p>
          </div>
          <Badge variant='outline'>{t('One level')}</Badge>
        </div>
        <div className='flex gap-2'>
          <Input
            value={invitationUrl}
            readOnly
            className='min-w-0 font-mono text-xs'
          />
          <CopyButton
            value={invitationUrl}
            variant='outline'
            tooltip={t('Copy invitation link')}
          />
        </div>
      </section>

      <section className='grid gap-3 rounded-md border p-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
        <div>
          <h3 className='font-semibold'>{t('Default customer pricing')}</h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {overall
              ? t('Current overall multiplier: {{value}}x', {
                  value: (overall.current_multiplier_bps / 10000).toFixed(4),
                })
              : t(
                  'No default rule is set. Platform pricing is used at 1.0000x.'
                )}
          </p>
        </div>
        <Button variant='outline' onClick={onPricing}>
          <SlidersHorizontal />
          {t('Edit pricing')}
        </Button>
      </section>

      <div className='grid gap-3 sm:grid-cols-3'>
        <ActionTile
          icon={CircleDollarSign}
          title={t('Convert earnings')}
          description={t('Move available commission to your API wallet.')}
          onClick={() => onAction('convert')}
        />
        <ActionTile
          icon={ArrowRightLeft}
          title={t('Send quota')}
          description={t('Transfer quota through preview and commit.')}
          onClick={() => onAction('transfer')}
        />
        <ActionTile
          icon={Ticket}
          title={t('Issue user codes')}
          description={t('Create one-time codes backed by escrowed quota.')}
          onClick={() => onAction('voucher')}
        />
      </div>
    </div>
  )
}

function ActionTile({
  icon: Icon,
  title,
  description,
  onClick,
}: {
  icon: typeof Ticket
  title: string
  description: string
  onClick: () => void
}) {
  return (
    <button
      type='button'
      onClick={onClick}
      className='hover:bg-muted/40 focus-visible:ring-ring flex min-h-28 items-start gap-3 rounded-md border p-4 text-left transition-colors focus-visible:ring-2 focus-visible:outline-none'
    >
      <Icon className='text-muted-foreground mt-0.5 size-4 shrink-0' />
      <span>
        <span className='block font-medium'>{title}</span>
        <span className='text-muted-foreground mt-1 block text-sm'>
          {description}
        </span>
      </span>
    </button>
  )
}

function CustomersPanel({
  page,
  onPricing,
  onPageChange,
}: {
  page: ResellerPage<ResellerCustomer>
  onPricing: (customer: ResellerCustomer) => void
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <DataSection
      title={t('Direct customers')}
      count={page.total}
      empty={page.items.length === 0}
      emptyText={t('No direct customers yet.')}
      page={page}
      onPageChange={onPageChange}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Customer')}</TableHead>
            <TableHead>{t('Group')}</TableHead>
            <TableHead>{t('Usage')}</TableHead>
            <TableHead>{t('Bound at')}</TableHead>
            <TableHead className='text-right'>{t('Pricing')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {page.items.map((customer) => (
            <TableRow key={customer.binding_id}>
              <TableCell>
                <div className='font-medium'>
                  {customer.display_name || customer.username}
                </div>
                <div className='text-muted-foreground text-xs'>
                  #{customer.customer_id} · {customer.username}
                </div>
              </TableCell>
              <TableCell>
                <Badge variant='outline'>{customer.group}</Badge>
              </TableCell>
              <TableCell>{formatQuota(customer.used_quota)}</TableCell>
              <TableCell>{formatTimestampToDate(customer.bound_at)}</TableCell>
              <TableCell className='text-right'>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  onClick={() => onPricing(customer)}
                  aria-label={t('Edit customer pricing')}
                >
                  <Settings2 />
                </Button>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataSection>
  )
}

function LedgerPanel({
  page,
  onPageChange,
}: {
  page: ResellerPage<ResellerLedgerItem>
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <DataSection
      title={t('Ledger entries')}
      count={page.total}
      empty={page.items.length === 0}
      emptyText={t('No ledger entries yet.')}
      page={page}
      onPageChange={onPageChange}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Type')}</TableHead>
            <TableHead>{t('Reference')}</TableHead>
            <TableHead>{t('Amount')}</TableHead>
            <TableHead>{t('Created at')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {page.items.map((item) => (
            <TableRow key={item.id}>
              <TableCell>
                <Badge variant='outline'>{t(item.kind)}</Badge>
              </TableCell>
              <TableCell className='max-w-64 truncate font-mono text-xs'>
                {item.reference}
              </TableCell>
              <TableCell>{formatQuota(item.amount_quota)}</TableCell>
              <TableCell>{formatTimestampToDate(item.created_at)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataSection>
  )
}

function TransfersPanel({
  page,
  onTransfer,
  onPageChange,
}: {
  page: ResellerPage<ResellerTransfer>
  onTransfer: () => void
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <DataSection
      title={t('Quota transfers')}
      count={page.total}
      action={
        <Button size='sm' onClick={onTransfer}>
          <ArrowRightLeft />
          {t('Send quota')}
        </Button>
      }
      empty={page.items.length === 0}
      emptyText={t('No transfers yet.')}
      page={page}
      onPageChange={onPageChange}
    >
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('Direction')}</TableHead>
            <TableHead>{t('Counterparty')}</TableHead>
            <TableHead>{t('Amount')}</TableHead>
            <TableHead>{t('Reference')}</TableHead>
            <TableHead>{t('Created at')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {page.items.map((item) => (
            <TableRow key={item.public_id}>
              <TableCell>
                {item.direction === 'sent' ? (
                  <Badge variant='outline'>
                    <ArrowUpRight />
                    {t('Sent')}
                  </Badge>
                ) : (
                  <Badge variant='secondary'>
                    <ArrowDownLeft />
                    {t('Received')}
                  </Badge>
                )}
              </TableCell>
              <TableCell>
                {item.counterparty_name || `#${item.counterparty_user_id}`}
              </TableCell>
              <TableCell>{item.amount}</TableCell>
              <TableCell className='font-mono text-xs'>
                {item.public_id}
              </TableCell>
              <TableCell>{formatTimestampToDate(item.created_at)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </DataSection>
  )
}

function VouchersPanel({
  vouchers,
  batches,
  onIssue,
  onReveal,
  onVoucherPageChange,
  onBatchPageChange,
}: {
  vouchers: ResellerPage<ResellerVoucher>
  batches: ResellerPage<ResellerVoucherBatch>
  onIssue: () => void
  onReveal: (publicId: string, batch: boolean) => void
  onVoucherPageChange: (page: number) => void
  onBatchPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-4'>
      <DataSection
        title={t('User code batches')}
        count={batches.total}
        action={
          <Button size='sm' onClick={onIssue}>
            <Ticket />
            {t('Issue codes')}
          </Button>
        }
        empty={batches.items.length === 0}
        emptyText={t('No user code batches yet.')}
        page={batches}
        onPageChange={onBatchPageChange}
      >
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Batch')}</TableHead>
              <TableHead>{t('Codes')}</TableHead>
              <TableHead>{t('Amount each')}</TableHead>
              <TableHead>{t('Note')}</TableHead>
              <TableHead className='text-right'>{t('Reveal')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {batches.items.map((batch) => (
              <TableRow key={batch.public_id}>
                <TableCell className='font-mono text-xs'>
                  {batch.public_id}
                </TableCell>
                <TableCell>{batch.count}</TableCell>
                <TableCell>{batch.amount}</TableCell>
                <TableCell className='max-w-64 truncate'>
                  {batch.note || '-'}
                </TableCell>
                <TableCell className='text-right'>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    onClick={() => onReveal(batch.public_id, true)}
                    aria-label={t('Reveal batch codes')}
                  >
                    <KeyRound />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataSection>
      <DataSection
        title={t('Individual user codes')}
        count={vouchers.total}
        empty={vouchers.items.length === 0}
        emptyText={t('No user codes yet.')}
        page={vouchers}
        onPageChange={onVoucherPageChange}
      >
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Code reference')}</TableHead>
              <TableHead>{t('Amount')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead>{t('Created at')}</TableHead>
              <TableHead className='text-right'>{t('Reveal')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {vouchers.items.map((voucher) => (
              <TableRow key={voucher.public_id}>
                <TableCell className='font-mono text-xs'>
                  {voucher.public_id}
                </TableCell>
                <TableCell>{voucher.amount}</TableCell>
                <TableCell>
                  <Badge
                    variant={voucher.redeemed_at ? 'secondary' : 'outline'}
                  >
                    {voucher.redeemed_at ? t('Redeemed') : t('Available')}
                  </Badge>
                </TableCell>
                <TableCell>
                  {formatTimestampToDate(voucher.created_at)}
                </TableCell>
                <TableCell className='text-right'>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    onClick={() => onReveal(voucher.public_id, false)}
                    aria-label={t('Reveal user code')}
                  >
                    <KeyRound />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </DataSection>
    </div>
  )
}

function SecurityPanel({
  security,
  receivePublicId,
  onAction,
  onRotate,
}: {
  security: ResellerSecurityStatus | null
  receivePublicId: string
  onAction: (action: ResellerActionKind) => void
  onRotate: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='divide-y rounded-md border'>
      <div className='grid gap-3 p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
        <div>
          <h3 className='font-medium'>{t('Quota password')}</h3>
          <p className='text-muted-foreground mt-1 text-sm'>
            {security?.configured
              ? t('Configured · version {{version}}', {
                  version: security.password_version,
                })
              : t('Not configured')}
          </p>
        </div>
        <div className='flex flex-wrap gap-2'>
          {security?.configured ? (
            <>
              <Button
                variant='outline'
                size='sm'
                onClick={() => onAction('password-change')}
              >
                <UserRoundCog />
                {t('Change')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                onClick={() => onAction('password-reset')}
              >
                <RotateCcwKey />
                {t('Reset')}
              </Button>
            </>
          ) : (
            <Button size='sm' onClick={() => onAction('password-set')}>
              <LockKeyhole />
              {t('Set password')}
            </Button>
          )}
        </div>
      </div>
      <div className='grid gap-3 p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
        <div className='min-w-0'>
          <h3 className='font-medium'>{t('Receive address')}</h3>
          <p className='text-muted-foreground mt-1 truncate font-mono text-xs'>
            {receivePublicId}
          </p>
        </div>
        <div className='flex gap-2'>
          <CopyButton
            value={receivePublicId}
            variant='outline'
            tooltip={t('Copy receive address')}
          />
          <Button variant='outline' size='sm' onClick={onRotate}>
            <RefreshCw />
            {t('Rotate')}
          </Button>
        </div>
      </div>
    </div>
  )
}

function DataSection({
  title,
  count,
  action,
  empty,
  emptyText,
  page,
  onPageChange,
  children,
}: {
  title: string
  count: number
  action?: React.ReactNode
  empty: boolean
  emptyText: string
  page?: ResellerPage<unknown>
  onPageChange?: (page: number) => void
  children: React.ReactNode
}) {
  return (
    <section className='overflow-hidden rounded-md border'>
      <div className='flex min-h-12 flex-wrap items-center justify-between gap-2 border-b px-3 py-2'>
        <div className='flex items-center gap-2'>
          <h3 className='font-medium'>{title}</h3>
          <Badge variant='secondary'>{count}</Badge>
        </div>
        {action}
      </div>
      {empty ? (
        <div className='text-muted-foreground grid min-h-40 place-items-center p-4 text-sm'>
          {emptyText}
        </div>
      ) : (
        children
      )}
      {page && onPageChange ? (
        <DataPagination page={page} onPageChange={onPageChange} />
      ) : null}
    </section>
  )
}

function DataPagination({
  page,
  onPageChange,
}: {
  page: ResellerPage<unknown>
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation()
  const pageCount = Math.max(1, Math.ceil(page.total / page.page_size))
  if (pageCount === 1 && page.page === 1) return null
  return (
    <div className='flex min-h-12 items-center justify-between gap-3 border-t px-3 py-2'>
      <span className='text-muted-foreground text-xs tabular-nums'>
        {t('Page {{page}} of {{total}}', {
          page: page.page,
          total: pageCount,
        })}
      </span>
      <div className='flex gap-1'>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={page.page <= 1}
          onClick={() => onPageChange(page.page - 1)}
          aria-label={t('Previous page')}
        >
          <ChevronLeft />
        </Button>
        <Button
          variant='outline'
          size='icon-sm'
          disabled={page.page >= pageCount}
          onClick={() => onPageChange(page.page + 1)}
          aria-label={t('Next page')}
        >
          <ChevronRight />
        </Button>
      </div>
    </div>
  )
}
